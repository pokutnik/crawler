package lib

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

type Task struct {
	location *url.URL
	Links    []*url.URL
	outDir   string
}

func NewTask(u *url.URL, out string) *Task {
	return &Task{
		location: u,
		outDir:   out,
	}
}

func (task *Task) Execute() {
	target_url := task.location.String()
	log.Printf("[task] ++ %v", target_url)
	// TODO: use golang.org/x/net/context/ctxhttp for cancelation
	resp, err := http.Get(target_url)

	if err != nil {
		// TODO: retry on error
		log.Printf("[error] skip '%v', due to: %v", target_url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[error] skip '%v', due to: %v", target_url, "Not 200 response")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if IsTextContent(ct) {
		// TODO: save to file
		contentReader := resp.Body
		wg := sync.WaitGroup{}
		if IsHTML(ct) {
			_reader, htmlW := io.Pipe()
			contentReader = _reader
			defer contentReader.Close()
			htmlR := ioutil.NopCloser(io.TeeReader(resp.Body, htmlW))
			wg.Add(1)
			go func() {
				defer htmlW.Close()
				defer htmlR.Close()
				log.Printf("[task] e+ %v", target_url)
				task.ExtractLinks(htmlR)
				log.Printf("[task] e- %v", target_url)
				wg.Done()
			}()
		}
		log.Printf("[task] s+ %v", target_url)
		task.SaveContent(contentReader)
		log.Printf("[task] s- %v", target_url)
		wg.Wait()
	} else {
		// TODO: cancel request
	}
	log.Printf("[task] -- %v", target_url)
}

func (task *Task) SaveContent(body io.Reader) {
	fileWriter := task.outFile()
	defer fileWriter.Close()
	n, err := io.Copy(fileWriter, body)
	if err != nil {
		log.Fatal("[error] Copy %v. Error: %v", task.location, err)
	}
	log.Printf("[task]    %v Size: %v", task.location, n)
}

func (task *Task) ExtractLinks(body io.Reader) {
	page := html.NewTokenizer(body)
	links := []string{}
	for {
		tokenType := page.Next()
		if tokenType == html.ErrorToken {
			break
		}
		token := page.Token()
		if tokenType == html.StartTagToken {
			switch token.DataAtom.String() {
			case "a":
				if relUrl := getAttr("href", token); relUrl != "" {
					links = append(links, relUrl)
				}
				break
			case "link":
				if relUrl := getAttr("href", token); relUrl != "" {
					links = append(links, relUrl)
				}
				break
			case "script":
				if relUrl := getAttr("src", token); relUrl != "" {
					links = append(links, relUrl)
				}
				break
			}
		}
	}
	for _, ref := range links {
		fullUrl, err := task.location.Parse(ref)
		fullUrl.Fragment = ""
		if err != nil {
			continue
		}
		task.Links = append(task.Links, fullUrl)
	}
	return
}

func getAttr(attr string, token html.Token) string {
	for _, attr := range token.Attr {
		if attr.Key == "href" {
			return attr.Val
		}
	}
	return ""
}

func (task *Task) outPath() (filename string) {
	// mapping URL to file system is not well defined, so using simplificatoin
	last_path := task.location.RequestURI()
	if strings.HasSuffix(last_path, "/") {
		last_path += "index.html"
	}

	filename = path.Join(task.outDir, task.location.Host, last_path)

	if path.Ext(filename) == "" {
		filename += ".html"
	}

	basepath := path.Dir(filename)
	err := os.MkdirAll(basepath, 0777)
	if err != nil {
		log.Fatal(err)
	}
	return
}

func (task *Task) outFile() io.WriteCloser {
	filename := task.outPath()
	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	return file
}

func IsTextContent(ct string) bool {
	if ct == "" {
		return false
	}

	if strings.HasPrefix(ct, "text/") {
		return true
	}
	if strings.HasPrefix(ct, "application/x-javascript") {
		return true
	}
	return false
}

func IsHTML(ct string) bool {
	return strings.HasPrefix(ct, "text/html")
}
