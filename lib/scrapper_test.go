package lib

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestScraper(t *testing.T) {
	const START_URL = "https://golang.org/pkg/"
	outDir, err := ioutil.TempDir("", ".scrapetest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outDir) // clean up
	s, err := New(START_URL, outDir, 10)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	s.WaitTillDone()
}
