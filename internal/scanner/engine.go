package scanner

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Fastdev75/xsearch/internal/httpclient"
	"github.com/Fastdev75/xsearch/internal/output"
	"github.com/Fastdev75/xsearch/internal/utils"
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

// Engine is the main scanning engine - optimized for speed and accuracy
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
	total     uint64 // Total URLs to scan for progress

	// Deduplication
	visited sync.Map

	// Output deduplication (for file output)
	outputURLs sync.Map

	// Discovered directories for recursive scanning
	directories    []string
	directoriesMux sync.Mutex

	// Multiple baseline detection for better soft 404 handling
	baselines []baseline

	// Soft 404 size tracking - detect when many responses have same size
	soft404Sizes    map[int64]int
	soft404SizesMux sync.Mutex

	// Filter maps for O(1) lookup
	filterCodes map[int]bool
	filterSizes map[int64]bool

	startTime time.Time
}

type baseline struct {
	hash string
	size int64
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
		config:       cfg,
		client:       httpclient.NewClient(&httpclient.Config{Timeout: cfg.Timeout, UserAgent: cfg.UserAgent}),
		printer:      output.NewPrinter(cfg.StatusCodes),
		writer:       writer,
		ctx:          ctx,
		cancel:       cancel,
		directories:  make([]string, 0, 100),
		baselines:    make([]baseline, 0, 5),
		soft404Sizes: make(map[int64]int),
		filterCodes:  filterCodes,
		filterSizes:  filterSizes,
	}
}

// Run starts the optimized 3-phase scanning process
func (e *Engine) Run() error {
	baseURL := e.normalizeURL(e.config.TargetURL)
	e.startTime = time.Now()

	// Print config
	utils.PrintInfo("Target: %s", baseURL)
	utils.PrintInfo("Threads: %d | Depth: %d | Recursive: %v", e.config.Threads, e.config.MaxDepth, e.config.Recursive)
	if len(e.config.Extensions) > 0 {
		utils.PrintInfo("Extensions: %s", strings.Join(e.config.Extensions, ", "))
	}

	// Multi-point calibration for better soft 404 detection
	e.calibrateMultiple(baseURL)

	fmt.Println(strings.Repeat("─", 70))

	// === PHASE 1: Fast directory discovery (HEAD requests) ===
	utils.PrintInfo("Phase 1: Directory Discovery (fast)")
	e.scanDirectoriesFast(baseURL, 0)

	// === PHASE 2: Recursive subdirectory discovery ===
	if e.config.Recursive && len(e.directories) > 0 {
		for depth := 1; depth <= e.config.MaxDepth; depth++ {
			select {
			case <-e.ctx.Done():
				return nil
			default:
			}

			// Get directories discovered at previous depth
			dirs := e.getDirectoriesAtDepth(depth - 1)
			if len(dirs) == 0 {
				break
			}

			utils.PrintInfo("Phase 2: Scanning %d directories at depth %d", len(dirs), depth)
			for _, dir := range dirs {
				select {
				case <-e.ctx.Done():
					return nil
				default:
				}
				e.scanDirectoriesFast(dir, depth)
			}
		}
	}

	// === PHASE 3: File discovery in all found directories ===
	if len(e.config.Extensions) > 0 {
		utils.PrintInfo("Phase 3: File Discovery (%d extensions)", len(e.config.Extensions))
		allDirs := e.getAllDirectories()
		// Add base URL to scan for files
		allDirs = append([]string{baseURL}, allDirs...)

		for _, dir := range allDirs {
			select {
			case <-e.ctx.Done():
				return nil
			default:
			}
			e.scanFiles(dir)
		}
	}

	return nil
}

// calibrateMultiple performs multiple calibration requests for better soft 404 detection
func (e *Engine) calibrateMultiple(baseURL string) {
	patterns := []string{
		"xsearch_%d_calibration",
		"nonexistent_%d_page",
		"random_%d_test_path",
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	hashCounts := make(map[string]int)
	sizeCounts := make(map[int64]int)

	for _, pattern := range patterns {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			randomURL := fmt.Sprintf("%s/%s", baseURL, fmt.Sprintf(p, time.Now().UnixNano()))
			result := httpclient.RequestWithBody(e.client, randomURL, e.config.UserAgent)
			if result.Error == nil && result.StatusCode != 0 {
				mu.Lock()
				hashCounts[result.BodyHash]++
				sizeCounts[result.Size]++
				e.baselines = append(e.baselines, baseline{hash: result.BodyHash, size: result.Size})
				mu.Unlock()
			}
		}(pattern)
	}
	wg.Wait()

	// Find most common hash and size for reporting
	var commonHash string
	var commonSize int64
	maxCount := 0
	for h, c := range hashCounts {
		if c > maxCount {
			maxCount = c
			commonHash = h
		}
	}
	maxCount = 0
	for s, c := range sizeCounts {
		if c > maxCount {
			maxCount = c
			commonSize = s
		}
	}

	if len(e.baselines) > 0 && commonHash != "" {
		utils.PrintInfo("Calibration: size=%d hash=%s (sampled %d)", commonSize, commonHash[:8], len(e.baselines))
	}
}

// scanDirectoriesFast performs fast directory discovery using HEAD requests
func (e *Engine) scanDirectoriesFast(basePath string, depth int) {
	basePath = strings.TrimRight(basePath, "/")

	// Build directory URLs only (no extensions)
	urls := e.buildDirectoryURLs(basePath, depth)
	if len(urls) == 0 {
		return
	}

	totalURLs := uint64(len(urls))
	atomic.StoreUint64(&e.total, totalURLs)
	startProcessed := atomic.LoadUint64(&e.processed)

	jobs := make(chan Job, e.config.Threads*4)
	results := make(chan Result, e.config.Threads*4)

	// Start workers with HEAD requests for speed
	var wg sync.WaitGroup
	for i := 0; i < e.config.Threads; i++ {
		wg.Add(1)
		go e.workerFast(jobs, results, &wg)
	}

	// Result handler
	var resultWg sync.WaitGroup
	resultWg.Add(1)
	go e.handleDirectoryResults(results, &resultWg, depth)

	// Progress reporter
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				// Clear progress line
				fmt.Printf("\r%s\r", strings.Repeat(" ", 60))
				return
			case <-ticker.C:
				current := atomic.LoadUint64(&e.processed) - startProcessed
				found := atomic.LoadUint64(&e.found)
				pct := float64(current) / float64(totalURLs) * 100
				if pct > 100 {
					pct = 100
				}
				fmt.Printf("\r[%.1f%%] %d/%d requests | Found: %d", pct, current, totalURLs, found)
			}
		}
	}()

	// Send jobs
	go func() {
	jobLoop:
		for _, u := range urls {
			select {
			case <-e.ctx.Done():
				break jobLoop
			case jobs <- Job{URL: u, Depth: depth}:
			}
		}
		close(jobs)
	}()

	wg.Wait()
	close(results)
	resultWg.Wait()
	close(progressDone)
}

// buildDirectoryURLs generates directory URLs only (no file extensions)
func (e *Engine) buildDirectoryURLs(basePath string, depth int) []string {
	var urls []string

	for _, word := range e.config.Words {
		word = strings.TrimSpace(word)
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.TrimPrefix(word, "/")

		// Skip words that look like files (have extensions)
		if strings.Contains(word, ".") {
			continue
		}

		fullURL := fmt.Sprintf("%s/%s", basePath, word)

		// Skip if visited
		if _, visited := e.visited.Load(fullURL); visited {
			continue
		}
		e.visited.Store(fullURL, depth)

		urls = append(urls, fullURL)

		// Also test with trailing slash for directory confirmation
		if e.config.AddSlash {
			slashURL := fullURL + "/"
			if _, visited := e.visited.Load(slashURL); !visited {
				e.visited.Store(slashURL, depth)
				urls = append(urls, slashURL)
			}
		}
	}

	return urls
}

// workerFast uses HEAD requests for faster directory discovery
func (e *Engine) workerFast(jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			// Use HEAD request first (faster)
			r := httpclient.HeadRequest(e.client, job.URL, e.config.UserAgent)

			// For successful responses, verify with GET to check soft 404
			needsVerification := r.Error == nil &&
				r.StatusCode != 404 &&
				!e.filterCodes[r.StatusCode] &&
				(r.StatusCode == 200 || r.StatusCode == 301 || r.StatusCode == 302 || r.StatusCode == 403)

			var bodyHash string
			var size int64 = r.Size

			if needsVerification {
				// Verify with GET request to check body hash
				fullResult := httpclient.RequestWithBody(e.client, job.URL, e.config.UserAgent)
				if fullResult.Error == nil {
					bodyHash = fullResult.BodyHash
					size = fullResult.Size
				}
			}

			select {
			case <-e.ctx.Done():
				return
			case results <- Result{
				URL:        r.URL,
				StatusCode: r.StatusCode,
				Size:       size,
				BodyHash:   bodyHash,
				Depth:      job.Depth,
				Error:      r.Error,
			}:
			}
		}
	}
}

// handleDirectoryResults processes directory scan results
func (e *Engine) handleDirectoryResults(results <-chan Result, wg *sync.WaitGroup, depth int) {
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

		// Skip server errors for recursive scanning (often false positives)
		if r.StatusCode >= 500 {
			continue
		}

		// Skip filtered sizes
		if e.filterSizes[r.Size] {
			continue
		}

		// Skip soft 404 (check against all baselines)
		if e.isSoft404(r.BodyHash, r.Size) {
			continue
		}

		// Dynamic soft 404 detection for 403/401 with repetitive sizes
		if e.trackSoft404Size(r.Size, r.StatusCode) {
			continue
		}

		// Determine if it's a directory
		isDir := e.isDirectory(r.URL, r.StatusCode)

		// Print result
		if e.printer.PrintResult(r.URL, r.StatusCode, r.Size, isDir, depth) {
			atomic.AddUint64(&e.found, 1)

			// Write to file - only reliable results, deduplicated
			if e.isReliableResult(r.StatusCode) && e.writer.IsEnabled() {
				e.writeUniqueURL(r.URL)
			}

			// Store directory for recursive scanning - only for successful responses
			// Don't recurse into 4xx errors as they're usually not real directories
			if isDir && (r.StatusCode == 200 || r.StatusCode == 301 || r.StatusCode == 302 || r.StatusCode == 307 || r.StatusCode == 308) {
				url := strings.TrimRight(r.URL, "/")
				e.directoriesMux.Lock()
				e.directories = append(e.directories, fmt.Sprintf("%d:%s", depth, url))
				e.directoriesMux.Unlock()
			}
		}
	}
}

// isReliableResult returns true if the status code indicates a reliable finding
func (e *Engine) isReliableResult(statusCode int) bool {
	// Only write truly valid results to output file
	return statusCode == 200 || statusCode == 301 || statusCode == 302 ||
		statusCode == 307 || statusCode == 308 || statusCode == 403 || statusCode == 401
}

// writeUniqueURL writes URL to output file, avoiding duplicates (normalizes trailing slash)
func (e *Engine) writeUniqueURL(url string) {
	// Normalize URL (remove trailing slash for deduplication)
	normalizedURL := strings.TrimRight(url, "/")

	// Check if already written
	if _, exists := e.outputURLs.LoadOrStore(normalizedURL, true); exists {
		return
	}

	// Write the original URL
	e.writer.WriteURL(url)
}

// scanFiles scans for files with extensions in a directory
func (e *Engine) scanFiles(basePath string) {
	basePath = strings.TrimRight(basePath, "/")

	urls := e.buildFileURLs(basePath)
	if len(urls) == 0 {
		return
	}

	totalURLs := uint64(len(urls))
	atomic.StoreUint64(&e.total, totalURLs)
	startProcessed := atomic.LoadUint64(&e.processed)
	startFound := atomic.LoadUint64(&e.found)

	jobs := make(chan Job, e.config.Threads*4)
	results := make(chan Result, e.config.Threads*4)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < e.config.Threads; i++ {
		wg.Add(1)
		go e.workerFiles(jobs, results, &wg)
	}

	// Result handler
	var resultWg sync.WaitGroup
	resultWg.Add(1)
	go e.handleFileResults(results, &resultWg)

	// Progress reporter
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				// Clear progress line
				fmt.Printf("\r%s\r", strings.Repeat(" ", 60))
				return
			case <-ticker.C:
				current := atomic.LoadUint64(&e.processed) - startProcessed
				found := atomic.LoadUint64(&e.found) - startFound
				pct := float64(current) / float64(totalURLs) * 100
				if pct > 100 {
					pct = 100
				}
				fmt.Printf("\r[%.1f%%] %d/%d requests | Found: %d", pct, current, totalURLs, found)
			}
		}
	}()

	// Send jobs
	go func() {
	jobLoop:
		for _, u := range urls {
			select {
			case <-e.ctx.Done():
				break jobLoop
			case jobs <- Job{URL: u, Depth: 0}:
			}
		}
		close(jobs)
	}()

	wg.Wait()
	close(results)
	resultWg.Wait()
	close(progressDone)
}

// buildFileURLs generates file URLs with extensions
func (e *Engine) buildFileURLs(basePath string) []string {
	var urls []string

	for _, word := range e.config.Words {
		word = strings.TrimSpace(word)
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		word = strings.TrimPrefix(word, "/")

		// Add each extension
		for _, ext := range e.config.Extensions {
			extURL := fmt.Sprintf("%s/%s.%s", basePath, word, ext)
			if _, visited := e.visited.Load(extURL); !visited {
				e.visited.Store(extURL, 0)
				urls = append(urls, extURL)
			}
		}
	}

	return urls
}

// workerFiles handles file discovery with GET requests
func (e *Engine) workerFiles(jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			// Use HEAD for speed, only GET if potentially interesting
			r := httpclient.HeadRequest(e.client, job.URL, e.config.UserAgent)

			var bodyHash string
			var size int64 = r.Size

			// Verify interesting results
			if r.Error == nil && r.StatusCode != 404 && !e.filterCodes[r.StatusCode] {
				fullResult := httpclient.RequestWithBody(e.client, job.URL, e.config.UserAgent)
				if fullResult.Error == nil {
					bodyHash = fullResult.BodyHash
					size = fullResult.Size
				}
			}

			select {
			case <-e.ctx.Done():
				return
			case results <- Result{
				URL:        r.URL,
				StatusCode: r.StatusCode,
				Size:       size,
				BodyHash:   bodyHash,
				Depth:      job.Depth,
				Error:      r.Error,
			}:
			}
		}
	}
}

// handleFileResults processes file scan results
func (e *Engine) handleFileResults(results <-chan Result, wg *sync.WaitGroup) {
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

		// Skip server errors (usually false positives)
		if r.StatusCode >= 500 {
			continue
		}

		// Skip filtered sizes
		if e.filterSizes[r.Size] {
			continue
		}

		// Skip soft 404
		if e.isSoft404(r.BodyHash, r.Size) {
			continue
		}

		// Dynamic soft 404 detection for 403/401 with repetitive sizes
		if e.trackSoft404Size(r.Size, r.StatusCode) {
			continue
		}

		// Files are not directories
		isDir := false

		// Print result
		if e.printer.PrintResult(r.URL, r.StatusCode, r.Size, isDir, 0) {
			atomic.AddUint64(&e.found, 1)

			// Write to file - only reliable results, deduplicated
			if e.isReliableResult(r.StatusCode) && e.writer.IsEnabled() {
				e.writeUniqueURL(r.URL)
			}
		}
	}
}

// isSoft404 checks if response matches any baseline (soft 404)
func (e *Engine) isSoft404(hash string, size int64) bool {
	// Check against calibration baselines
	for _, b := range e.baselines {
		// Match by hash
		if hash != "" && b.hash == hash {
			return true
		}
		// Match by exact size (common for error pages)
		if b.size > 0 && size == b.size {
			return true
		}
	}
	return false
}

// trackSoft404Size tracks response sizes for dynamic soft 404 detection
// Returns true if this size has been seen too many times (likely soft 404)
func (e *Engine) trackSoft404Size(size int64, statusCode int) bool {
	// Only track 403 and 401 responses - these are often soft 404s
	if statusCode != 403 && statusCode != 401 {
		return false
	}

	// Very small responses are often error pages
	if size < 100 {
		e.soft404SizesMux.Lock()
		e.soft404Sizes[size]++
		count := e.soft404Sizes[size]
		e.soft404SizesMux.Unlock()

		// If we've seen this exact size more than 10 times, it's likely a soft 404
		if count > 10 {
			return true
		}
	}
	return false
}


// getDirectoriesAtDepth returns directories found at a specific depth
func (e *Engine) getDirectoriesAtDepth(depth int) []string {
	e.directoriesMux.Lock()
	defer e.directoriesMux.Unlock()

	prefix := fmt.Sprintf("%d:", depth)
	var dirs []string
	for _, d := range e.directories {
		if strings.HasPrefix(d, prefix) {
			dirs = append(dirs, strings.TrimPrefix(d, prefix))
		}
	}
	return dirs
}

// getAllDirectories returns all discovered directories
func (e *Engine) getAllDirectories() []string {
	e.directoriesMux.Lock()
	defer e.directoriesMux.Unlock()

	var dirs []string
	seen := make(map[string]bool)
	for _, d := range e.directories {
		parts := strings.SplitN(d, ":", 2)
		if len(parts) == 2 && !seen[parts[1]] {
			seen[parts[1]] = true
			dirs = append(dirs, parts[1])
		}
	}

	// Sort for consistent output
	sort.Strings(dirs)
	return dirs
}

// isDirectory determines if a path is likely a directory
func (e *Engine) isDirectory(url string, statusCode int) bool {
	// Redirects typically indicate directories
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

	// Print directories found
	dirs := e.getAllDirectories()
	if len(dirs) > 0 {
		utils.PrintSuccess("Directories found: %d", len(dirs))
	}

	if e.writer.IsEnabled() {
		utils.PrintSuccess("Saved to: %s", e.writer.GetPath())
	}
}
