package lib

import (
	"log"
	"net/url"
	"strings"
	"sync"
)

type Scraper struct {
	Root      *url.URL
	FetchQ    chan *Task
	DoneQ     chan *Task
	ReadyQ    chan *Task
	co_added  map[string]bool
	co_done   map[string]bool
	wgWorkers sync.WaitGroup
	outDir    string
	nWorkers  uint
}

// create new scraper
func New(root string, outDir string, nWorkers uint) (*Scraper, error) {
	Root, err := url.Parse(root)
	if err != nil {
		return nil, err
	}

	s := &Scraper{
		// settings
		outDir:   outDir,
		Root:     Root,
		nWorkers: nWorkers,

		// for workers
		FetchQ: make(chan *Task, 1),

		// for coordinators
		DoneQ:    make(chan *Task, 1),
		ReadyQ:   make(chan *Task, 1),
		co_added: make(map[string]bool),
		co_done:  make(map[string]bool),
	}

	return s, nil
}

// Start scraper
func (s *Scraper) Start() {
	s.wgWorkers.Add(1)
	go s.runCoordinator()

	for i := uint(0); i < s.nWorkers; i++ {
		s.wgWorkers.Add(1)
		go s.runWorker()
	}
}

// Block till scraper is done
func (s *Scraper) WaitTillDone() {
	s.wgWorkers.Wait()
}

func (s *Scraper) shouldFetch(l *url.URL) bool {
	sameHost := l.Host == s.Root.Host
	return sameHost && strings.HasPrefix(l.Path, s.Root.Path)
}

// Mark task as done, return true if all tasks are done
// safe to call only form coordinator goroutine
func (s *Scraper) coMarkDone(task *Task) bool {
	key := task.location.String()
	s.co_done[key] = true
	allDone := len(s.co_added) == len(s.co_done)
	return allDone
}

// Mark url as new
// safe to call only form coordinator goroutine
func (s *Scraper) coMaybeAddNew(l *url.URL) bool {
	key := l.String()
	_, ok := s.co_added[key]
	if !ok {
		s.co_added[key] = true
	}
	return !ok
}

func (s *Scraper) sendDone(task *Task) {
	s.DoneQ <- task
}
func (s *Scraper) sendReady(task *Task) {
	s.ReadyQ <- task
}

func (s *Scraper) runCoordinator() {
	defer s.wgWorkers.Done()
	log.Print("[coord] starting...")

	s.coMaybeAddNew(s.Root)
	s.FetchQ <- NewTask(s.Root, s.outDir)
	// TODO: Add restore option

	for {
		select {
		case task, ok := <-s.ReadyQ:
			if !ok {
				s.ReadyQ = nil
			} else {
				log.Printf("[coord] done: %v", task.location)

				newLinks := make([]*url.URL, 0)
				for _, link := range task.Links {
					if s.shouldFetch(link) && s.coMaybeAddNew(link) {
						newLinks = append(newLinks, link)
					}
				}
				if len(newLinks) > 0 {
					log.Printf("[coord] found for %v -> %v", task.location, newLinks)
				}

				// TODO: replace FetchQ with some sort of persistent queue
				go func(newLinks []*url.URL, task *Task) {
					for _, link := range newLinks {
						s.FetchQ <- NewTask(link, s.outDir)
					}
					s.sendDone(task)
				}(newLinks, task)
			}
		case task, ok := <-s.DoneQ:
			if !ok {
				s.DoneQ = nil
			} else {
				if s.coMarkDone(task) {
					s.scrapingDone()
				}
			}
		}
		if s.DoneQ == nil && s.ReadyQ == nil {
			break
		}
	}

	log.Print("[coord] stopping...")
}

func (s *Scraper) scrapingDone() {
	log.Printf("[info] scrapingDone shuting down")
	close(s.FetchQ)
	close(s.ReadyQ)
	close(s.DoneQ)
}

func (s *Scraper) runWorker() {
	defer s.wgWorkers.Done()
	log.Printf("[worker] started...")
	for task := range s.FetchQ {
		task.Execute()
		s.sendReady(task)
	}
	log.Printf("[worker] exit...")
}
