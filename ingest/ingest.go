package ingest

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sync"

	"github.com/RediSearch/RediSearchBenchmark/index"
)

// DocumentReader implements parsing a data source and yielding documents
type DocumentReader interface {
	Read(io.Reader, chan index.Document, int, index.Index) error
}

func walkDir(path string, pattern string, ch chan string) {

	files, err := ioutil.ReadDir(path)

	if err != nil {
		log.Printf("Could not read path %s: %s", path, err)
		panic(err)
	}

	for _, file := range files {
		fullpath := filepath.Join(path, file.Name())
		if file.IsDir() {
			walkDir(fullpath, pattern, ch)
			continue
		}

		if match, err := filepath.Match(pattern, file.Name()); err == nil {

			if match {
				log.Println("Reading ", fullpath)
				ch <- fullpath
			}
		} else {
			panic(err)
		}

	}
}

type Stats struct {
	TotalDocs             int64
	CurrentWindowDocs     int
	CurrentWindowDuration time.Duration
	CurrentWindowRate     float64
	CurrentWindowLatency  time.Duration
}

func ngrams(words []string, size int, count map[string]uint32) {

	offset := int(math.Floor(float64(size / 2)))

	max := len(words)
	for i := range words {
		if i < offset || i+size-offset > max {
			continue
		}
		gram := strings.Join(words[i-offset:i+size-offset], " ")
		count[gram] += uint32(size)
	}

}

// ReadDir reads a complete directory and feeds each file it finds to a document reader
func ReadDir(dirName string, pattern string, r DocumentReader, idx index.Index, ac index.Autocompleter,
	opts interface{}, chunk int, workers int, conns int, stats chan Stats, maxDocsToRead int) {
	filech := make(chan string, 100)
	go func() {
		defer close(filech)
		walkDir(dirName, pattern, filech)
	}()

	doch := make(chan index.Document, chunk)
	countch := make(chan time.Duration, chunk*workers)
	// start the independent idexing workers
	wg := sync.WaitGroup{}
	go func() {
		for i := 0; i < conns; i++ {
			wg.Add(1)
			go func(doch chan index.Document, countch chan time.Duration) {
				for doc := range doch {
					if doc.Id != "" {
						//fmt.Println(doc)
						st := time.Now()
						idx.Index([]index.Document{doc}, opts)
						dur := time.Since(st)
						countch <- dur

						// words := strings.Fields(strings.ToLower(doc.Properties["body"].(string)))
						// grams := map[string]uint32{}
						// ngrams(words, 1, grams)
						// ngrams(words, 2, grams)
						// ngrams(words, 3, grams)
						// suggestions := make(index.SuggestionList, 0, len(grams))
						// for gr, count := range grams {
						// 	suggestions = append(suggestions, index.Suggestion{Term: gr, Score: float64(count)})
						// }
						// suggestions.Sort()
						// if len(suggestions) > 10 {
						// 	suggestions = suggestions[:10]
						// }
						// ac.AddTerms(suggestions...)

					}
				}
				wg.Done()
			}(doch, countch)

		}
		wg.Wait()
	}()
	// start the file reader workers
	for i := 0; i < workers; i++ {
		go func(filech chan string, doch chan index.Document) {
			for file := range filech {
				fp, err := os.Open(file)
				if err != nil {
					log.Println(err)
				} else {
					if err = r.Read(fp, doch, maxDocsToRead, idx); err != nil {
						log.Printf("Error reading %s: %s", file, err)
					}
				}
				fp.Close()
			}
		}(filech, doch)
	}

	stt := Stats{
		CurrentWindowDocs:     0,
		TotalDocs:             0,
		CurrentWindowRate:     0,
		CurrentWindowDuration: 0,
		CurrentWindowLatency:  0,
	}

	st := time.Now()
	var totalLatency time.Duration
	for rtt := range countch {
		stt.TotalDocs++
		stt.CurrentWindowDocs++
		totalLatency += rtt

		if time.Since(st) > 200*time.Millisecond {
			stt.CurrentWindowDuration = time.Since(st)
			stt.CurrentWindowRate = float64(stt.CurrentWindowDocs) / (float64(stt.CurrentWindowDuration.Seconds()))
			stt.CurrentWindowLatency = totalLatency / (1 + time.Duration(stt.CurrentWindowDocs))
			//dtrate := float32(dt) / (float32(time.Since(st).Seconds())) / float32(1024*1024)
			fmt.Println(stt.TotalDocs, "docs done, avg latency:", stt.CurrentWindowLatency, " rate: ", stt.CurrentWindowRate, "d/s")
			st = time.Now()
			if stats != nil {
				stats <- stt
			}
			stt.CurrentWindowDocs = 0
			totalLatency = 0

		}
	}
	fmt.Println("Done!")
}

// IngestDocuments ingests documents into an index using a DocumentReader
func ReadFile(fileName string, r DocumentReader, idx index.Index, ac index.Autocompleter,
	opts interface{}, chunk int, maxDocsToRead int) error {

	var wg sync.WaitGroup

	// open the file
	fp, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fp.Close()
	ch := make(chan index.Document, chunk)
	// run the reader and let it spawn a goroutine
	if err := r.Read(fp, ch, maxDocsToRead, idx); err != nil {
		return err
	}

	terms := make([]index.Suggestion, chunk*2)

	//freqs := map[string]int{}
	st := time.Now()

	nterms := 0
	i := 0
	n := 0
	dt := 0
	totalDt := 0
	doch := make(chan index.Document, 100)
	for w := 0; w < 200; w++ {
		wg.Add(1)
		go func(doch chan index.Document) {
			defer wg.Done()
			for doc := range doch {
				if doc.Id != "" {
					idx.Index([]index.Document{doc}, opts)
				}
			}
		}(doch)
	}
	for doc := range ch {

		//docs[i%chunk] = doc

		if doc.Score > 0 && ac != nil {

			//			words := strings.Fields(strings.ToLower(doc.Properties["body"].(string)))
			//			for _, w := range words {
			//				for i := 2; i < len(w) && i < 5; i++ {
			//					freqs[w[:i]] += 1
			//				}
			//			}

			terms[nterms] = index.Suggestion{
				strings.ToLower(doc.Properties["title"].(string)),
				float64(doc.Score),
			}
			nterms++

			if nterms == chunk {

				//				for k, v := range freqs {
				//					fmt.Printf("%d %s\n", v, k)
				//				}
				//				os.Exit(0)
				/*if err := ac.AddTerms(terms...); err != nil {
					return err
				}*/
				nterms = 0
			}

		}
		if doc.Score == 0 {
			doc.Score = 0.0000001
		}

		for k, v := range doc.Properties {
			switch s := v.(type) {
			case string:
				dt += len(s) + len(k)
				totalDt += len(s) + len(k)
			}
		}

		i++
		n++
		doch <- doc
		// if i%chunk == 0 {
		// 	//var _docs []index.Document
		// 	for _, d := range docs {
		// 		doch <- d
		// 	}

		// }

		// print report every CHUNK documents
		if i%chunk == 0 {
			rate := float32(n) / (float32(time.Since(st).Seconds()))
			dtrate := float32(dt) / (float32(time.Since(st).Seconds())) / float32(1024*1024)
			fmt.Println(i, "rate: ", rate, "d/s. data rate: ", dtrate, "MB/s", "total data ingested", float32(totalDt)/float32(1024*1024))
			st = time.Now()
			n = 0
			dt = 0
		}
	}

	close(doch)

	wg.Wait()
	return nil
}
