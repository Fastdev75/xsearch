package scanner

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"xsearch/internal/httpclient"
	"xsearch/internal/output"
	"xsearch/internal/utils"
)

// Config holds scanner configuration
type Config struct {
	TargetURL    string
	Words        []string
	Threads      int
	Timeout      time.Duration
	UserAgent    string
	Extensions   []string
	Recursive    bool
	MaxDepth     int
	AddSlash     bool
	FilterCodes  []int
	ExcludeSizes []int64
	StatusCodes  []int
}

// Engine is the main scanning engine
type Engine struct {
	config  *Config
	client  *http.Client
	printer *output.Printer
	writer  *output.Writer
	ctx     context.Context
	cancel  context.CancelFunc

	// Stats (atomic)
	processed uint64
	found     uint64
	errors    uint64

	// Deduplication
	visited sync.Map

	// Recursive queue
	queue   chan string
	queueWg sync.WaitGroup

	// Soft 404 detection
	baseline struct {
		hash string
		size int64
	}

	// Filter maps for O(1) lookup
	filterCodes map[int]bool
	filterSizes map[int64]bool

	startTime time.Time
}

// NewEngine creates a new scanner engine
func NewEngine(cfg *Config, writer *output.Writer) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	// Build filter maps
	filterCodes := make(map[int]bool)
	for _, c := range cfg.FilterCodes {
		filterCodes[c] = true
	}
	filterSizes := make(map[int64]bool)
	for _, s := range cfg.ExcludeSizes {
		filterSizes[s] = true
	}

	return &Engine{
		config:      cfg,
		client:      httpclient.NewClient(&httpclient.Config{Timeout: cfg.Timeout, UserAgent: cfg.UserAgent}),
		printer:     output.NewPrinter(cfg.StatusCodes),
		writer:      writer,
		ctx:         ctx,
		cancel:      cancel,
		queue:       make(chan string, 1000),
		filterCodes: filterCodes,
		filterSizes: filterSizes,
	}
}

// Run starts the scanning process
func (e *Engine) Run() error {
	baseURL := e.normalizeURL(e.config.TargetURL)
	e.startTime = time.Now()

	// Print config
	utils.PrintInfo("Target: %s", baseURL)
	utils.PrintInfo("Threads: %d | Depth: %d | Recursive: %v", e.config.Threads, e.config.MaxDepth, e.config.Recursive)
	if len(e.config.Extensions) > 0 {
		utils.PrintInfo("Extensions: %s", strings.Join(e.config.Extensions, ", "))
	}

	// Calibrate soft 404
	e.calibrate(baseURL)

	fmt.Println(strings.Repeat("─", 70))

	// Start queue processor for recursive scanning
	if e.config.Recursive {
		e.queueWg.Add(1)
		go e.processQueue()
	}

	// Initial scan
	e.scanPath(baseURL, 0)

	// Wait for recursive queue to empty
	if e.config.Recursive {
		close(e.queue)
		e.queueWg.Wait()
	}

	return nil
}

// calibrate detects soft 404 baseline
func (e *Engine) calibrate(baseURL string) {
	randomURL := fmt.Sprintf("%s/xsearch_%d_calibration_test", baseURL, time.Now().UnixNano())
	result := httpclient.RequestWithBody(e.client, randomURL, e.config.UserAgent)
	if result.Error == nil {
		e.baseline.hash = result.BodyHash
		e.baseline.size = result.Size
		utils.PrintInfo("Calibration: size=%d hash=%s", e.baseline.size, e.baseline.hash[:8])
	}
}

// processQueue handles recursive directory scanning
func (e *Engine) processQueue() {
	defer e.queueWg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case path, ok := <-e.queue:
			if !ok {
				return
			}
			// Extract depth from visited map
			if v, ok := e.visited.Load(path); ok {
				if depth, ok := v.(int); ok && depth < e.config.MaxDepth {
					utils.PrintInfo("Scanning: %s (depth %d)", path, depth+1)
					e.scanPath(path, depth+1)
				}
			}
		}
	}
}

// scanPath scans a single path with all words
func (e *Engine) scanPath(basePath string, depth int) {
	urls := e.buildURLs(basePath, depth)
	if len(urls) == 0 {
		return
	}

	jobs := make(chan Job, e.config.Threads*2)
	results := make(chan Result, e.config.Threads*2)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < e.config.Threads; i++ {
		wg.Add(1)
		go e.worker(jobs, results, &wg)
	}

	// Result handler
	var resultWg sync.WaitGroup
	resultWg.Add(1)
	go e.handleResults(results, &resultWg, depth)

	// Send jobs
	go func() {
	urlLoop:
		for _, u := range urls {
			select {
			case <-e.ctx.Done():
				break urlLoop
			case jobs <- Job{URL: u.url, Depth: depth}:
			}
		}
		close(jobs)
	}()

	wg.Wait()
	close(results)
	resultWg.Wait()
}

type urlEntry struct {
	url   string
	isDir bool
}

// buildURLs generates URLs to scan
func (e *Engine) buildURLs(basePath string, depth int) []urlEntry {
	basePath = strings.TrimRight(basePath, "/")
	var urls []urlEntry

	for _, word := range e.config.Words {
		word = strings.TrimSpace(word)
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.TrimPrefix(word, "/")

		fullURL := fmt.Sprintf("%s/%s", basePath, word)

		// Skip if visited
		if _, visited := e.visited.Load(fullURL); visited {
			continue
		}
		e.visited.Store(fullURL, depth)

		urls = append(urls, urlEntry{url: fullURL, isDir: true})

		// Add slash version for directory detection
		if e.config.AddSlash {
			slashURL := fullURL + "/"
			if _, visited := e.visited.Load(slashURL); !visited {
				e.visited.Store(slashURL, depth)
				urls = append(urls, urlEntry{url: slashURL, isDir: true})
			}
		}

		// Add extensions
		for _, ext := range e.config.Extensions {
			extURL := fmt.Sprintf("%s/%s.%s", basePath, word, ext)
			if _, visited := e.visited.Load(extURL); !visited {
				e.visited.Store(extURL, depth)
				urls = append(urls, urlEntry{url: extURL, isDir: false})
			}
		}
	}

	return urls
}

// worker processes HTTP requests
func (e *Engine) worker(jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			r := httpclient.RequestWithBody(e.client, job.URL, e.config.UserAgent)
			select {
			case <-e.ctx.Done():
				return
			case results <- Result{
				URL:        r.URL,
				StatusCode: r.StatusCode,
				Size:       r.Size,
				BodyHash:   r.BodyHash,
				Depth:      job.Depth,
				Error:      r.Error,
			}:
			}
		}
	}
}

// handleResults processes scan results
func (e *Engine) handleResults(results <-chan Result, wg *sync.WaitGroup, depth int) {
	defer wg.Done()

	for r := range results {
		atomic.AddUint64(&e.processed, 1)

		if r.Error != nil {
			atomic.AddUint64(&e.errors, 1)
			continue
		}

		// Skip 404 and filtered codes
		if r.StatusCode == 404 || e.filterCodes[r.StatusCode] {
			continue
		}

		// Skip filtered sizes
		if e.filterSizes[r.Size] {
			continue
		}

		// Skip soft 404 (same hash as baseline - works for any status code)
		if e.baseline.hash != "" && r.BodyHash == e.baseline.hash {
			continue
		}

		// Determine if directory
		isDir := e.isDirectory(r.URL, r.StatusCode)

		// Print result
		if e.printer.PrintResult(r.URL, r.StatusCode, r.Size, isDir, depth) {
			atomic.AddUint64(&e.found, 1)

			// Write to file
			if output.IsInteresting(r.StatusCode) && e.writer.IsEnabled() {
				e.writer.WriteResult(r.URL, r.StatusCode, r.Size, isDir)
			}

			// Queue for recursive scanning
			if e.config.Recursive && isDir && depth < e.config.MaxDepth {
				if r.StatusCode == 200 || r.StatusCode == 301 || r.StatusCode == 302 || r.StatusCode == 403 {
					// Store depth for later use
					url := strings.TrimRight(r.URL, "/")
					e.visited.Store(url, depth)
					select {
					case e.queue <- url:
					default:
						// Queue full, skip
					}
				}
			}
		}
	}
}

// isDirectory determines if a path is likely a directory
func (e *Engine) isDirectory(url string, statusCode int) bool {
	// Redirects indicate directories
	if statusCode == 301 || statusCode == 302 || statusCode == 307 || statusCode == 308 {
		return true
	}
	// URL ends with slash
	if strings.HasSuffix(url, "/") {
		return true
	}
	// No extension in last path segment
	path := strings.TrimRight(url, "/")
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		lastPart := path[idx+1:]
		if !strings.Contains(lastPart, ".") {
			return true
		}
	}
	return false
}

// normalizeURL ensures proper URL format
func (e *Engine) normalizeURL(url string) string {
	url = strings.TrimRight(url, "/")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return url
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
	errors := atomic.LoadUint64(&e.errors)

	fmt.Println(strings.Repeat("─", 70))
	utils.PrintInfo("Completed in %s", duration.Round(time.Millisecond))
	utils.PrintInfo("Requests: %d | Found: %d | Errors: %d", processed, found, errors)

	if e.writer.IsEnabled() {
		utils.PrintSuccess("Saved to: %s", e.writer.GetPath())
	}
}
