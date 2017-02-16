package lib

import (
	"log"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Scraper struct {
	Root      *url.URL
	FetchQ    chan *Task
	ReadyQ    chan *Task
	co_added  map[string]bool
	co_done   map[string]bool
	co_ready  []*url.URL
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

func (s *Scraper) sendReady(task *Task) {
	s.ReadyQ <- task
}

func (s *Scraper) runCoordinator() {
	defer s.wgWorkers.Done()
	log.Print("[coord] starting...")
	statusTicker := time.NewTicker(time.Millisecond * 500).C

	s.coMaybeAddNew(s.Root)
	s.co_ready = append(s.co_ready, s.Root)
	nextTask, out := s.coNextTask()
	// TODO: Add restore option

	for {
		select {
		case out <- nextTask:
			s.coShrink()
			nextTask, out = s.coNextTask()
		case task, ok := <-s.ReadyQ:
			if !ok {
				s.ReadyQ = nil
			} else {
				log.Printf("[coord] done: %v", task.location)

				for _, link := range task.Links {
					if s.shouldFetch(link) && s.coMaybeAddNew(link) {
						s.co_ready = append(s.co_ready, link)
					}
				}
				nextTask, out = s.coNextTask()
				if s.coMarkDone(task) {
					s.scrapingDone()
				}
			}
		case <-statusTicker:
			s.logStatus()
		}
		if s.ReadyQ == nil {
			break
		}
	}

	log.Print("[coord] stopping...")
	runtime.GC()
	s.logStatus()
}

func (s *Scraper) coShrink() {
	last := len(s.co_ready) - 1
	s.co_ready = s.co_ready[:last]
}
func (s *Scraper) coNextTask() (*Task, chan *Task) {
	if len(s.co_ready) == 0 {
		return nil, nil
	}
	link := s.co_ready[len(s.co_ready)-1]
	return NewTask(link, s.outDir), s.FetchQ
}

func (s *Scraper) scrapingDone() {
	log.Printf("[info] scrapingDone shuting down")
	close(s.FetchQ)
	close(s.ReadyQ)
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

func (s *Scraper) logStatus() {
	log.Printf("[debug] added: %v\tdone: %v\tready: %v", len(s.co_added), len(s.co_done), len(s.co_ready))
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	log.Printf("[mem] %v", mem.Alloc)
}
