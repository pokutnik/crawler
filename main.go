package main

import (
	"flag"
	"log"

	"github.com/pokutnik/crawler/lib"
)

var rootURL = flag.String("url", "https://golang.org/pkg/", "full URL to start crawling from")
var outDir = flag.String("out", "/tmp/crawl", "directory to store files")
var nWorkers = flag.Uint("n", 16, "number of dowload workers")

func main() {
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	s, err := lib.New(*rootURL, *outDir, *nWorkers)
	if err != nil {
		log.Fatal(err)
	}

	s.Start()
	s.WaitTillDone()
}
