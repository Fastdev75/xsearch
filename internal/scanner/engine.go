package scanner

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mcauet/xsearch/internal/httpclient"
	"github.com/mcauet/xsearch/internal/output"
	"github.com/mcauet/xsearch/internal/utils"
)

// Config holds scanner configuration
type Config struct {
	TargetURL   string
	Words       []string
	Threads     int
	Timeout     time.Duration
	UserAgent   string
	StatusCodes []int
	Extensions  []string
}

// Engine is the main scanning engine
type Engine struct {
	config    *Config
	client    *http.Client
	printer   *output.Printer
	writer    *output.Writer
	ctx       context.Context
	cancel    context.CancelFunc
	processed uint64
	found     uint64
	total     uint64
	startTime time.Time
}

// NewEngine creates a new scanner engine
func NewEngine(cfg *Config, writer *output.Writer) *Engine {
	httpCfg := &httpclient.Config{
		Timeout:         cfg.Timeout,
		FollowRedirects: false,
		UserAgent:       cfg.UserAgent,
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Engine{
		config:  cfg,
		client:  httpclient.NewClient(httpCfg),
		printer: output.NewPrinter(cfg.StatusCodes),
		writer:  writer,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// normalizeURL ensures the URL has a proper format
func normalizeURL(url string) string {
	url = strings.TrimRight(url, "/")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return url
}

// buildURLs generates all URLs to scan
func (e *Engine) buildURLs() []string {
	baseURL := normalizeURL(e.config.TargetURL)
	var urls []string

	for _, word := range e.config.Words {
		word = strings.TrimPrefix(word, "/")

		// Add base path
		urls = append(urls, fmt.Sprintf("%s/%s", baseURL, word))

		// Add extensions if configured
		for _, ext := range e.config.Extensions {
			ext = strings.TrimPrefix(ext, ".")
			urls = append(urls, fmt.Sprintf("%s/%s.%s", baseURL, word, ext))
		}
	}

	return urls
}

// Run starts the scanning process
func (e *Engine) Run() error {
	urls := e.buildURLs()
	e.total = uint64(len(urls))
	e.startTime = time.Now()

	utils.PrintInfo("Target: %s", normalizeURL(e.config.TargetURL))
	utils.PrintInfo("Threads: %d", e.config.Threads)
	utils.PrintInfo("Wordlist entries: %d", len(e.config.Words))
	utils.PrintInfo("Total requests: %d", e.total)
	fmt.Println(strings.Repeat("-", 60))

	// Create channels
	jobs := make(chan Job, e.config.Threads*2)
	results := make(chan Result, e.config.Threads*2)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < e.config.Threads; i++ {
		wg.Add(1)
		worker := NewWorker(i, e.client, e.config.UserAgent, jobs, results, &wg)
		go worker.Start()
	}

	// Start result handler
	var resultWg sync.WaitGroup
	resultWg.Add(1)
	go e.handleResults(results, &resultWg)

	// Send jobs
	go func() {
		for _, url := range urls {
			select {
			case <-e.ctx.Done():
				break
			case jobs <- Job{URL: url}:
			}
		}
		close(jobs)
	}()

	// Wait for workers to finish
	wg.Wait()
	close(results)

	// Wait for result handler to finish
	resultWg.Wait()

	return nil
}

// handleResults processes scan results
func (e *Engine) handleResults(results <-chan Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for result := range results {
		atomic.AddUint64(&e.processed, 1)

		if result.Error != nil {
			continue
		}

		// Print to terminal if status code should be shown
		if e.printer.PrintResult(result.URL, result.StatusCode, result.Size) {
			atomic.AddUint64(&e.found, 1)

			// Write to file if status code is interesting (URL only, no status code)
			if output.IsInteresting(result.StatusCode) && e.writer.IsEnabled() {
				e.writer.WriteURL(result.URL)
			}
		}
	}
}

// Stop gracefully stops the scanner
func (e *Engine) Stop() {
	e.cancel()
}

// PrintStats prints final statistics
func (e *Engine) PrintStats() {
	duration := time.Since(e.startTime)
	processed := atomic.LoadUint64(&e.processed)
	found := atomic.LoadUint64(&e.found)

	fmt.Println(strings.Repeat("-", 60))
	utils.PrintInfo("Scan completed in %s", duration.Round(time.Millisecond))
	utils.PrintInfo("Requests: %d | Found: %d", processed, found)

	if e.writer.IsEnabled() {
		utils.PrintSuccess("Results saved to: %s", e.writer.GetPath())
	}
}

// GetContext returns the engine's context
func (e *Engine) GetContext() context.Context {
	return e.ctx
}
