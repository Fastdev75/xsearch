package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Fastdev75/xsearch/internal/output"
	"github.com/Fastdev75/xsearch/internal/scanner"
	"github.com/Fastdev75/xsearch/internal/utils"
	"github.com/Fastdev75/xsearch/internal/wordlist"
)

const version = "2.0.0"

func main() {
	// Essential flags only
	targetURL := flag.String("u", "", "Target URL (required)")
	wordlistPath := flag.String("w", "", "Custom wordlist path")
	outputFile := flag.String("o", "", "Output file")
	threads := flag.Int("t", 50, "Threads")
	extensions := flag.String("x", "", "Extensions (e.g., php,html,js)")

	// Simple toggles
	noRecursive := flag.Bool("nr", false, "Disable recursive mode")
	depth := flag.Int("d", 3, "Max depth")

	// Filtering (advanced)
	filterCodes := flag.String("fc", "", "Filter status codes (e.g., 403,500)")
	filterSize := flag.String("fs", "", "Filter by size")

	// Display options
	silent := flag.Bool("q", false, "Quiet mode (no banner)")
	showVersion := flag.Bool("v", false, "Version")
	showHelp := flag.Bool("h", false, "Help")

	flag.Parse()

	if *showVersion {
		fmt.Printf("xsearch v%s\n", version)
		os.Exit(0)
	}

	if *showHelp || *targetURL == "" {
		printHelp()
		os.Exit(0)
	}

	if !*silent {
		utils.Banner()
	}

	// Parse extensions
	var exts []string
	if *extensions != "" {
		for _, ext := range strings.Split(*extensions, ",") {
			ext = strings.TrimSpace(strings.TrimPrefix(ext, "."))
			if ext != "" {
				exts = append(exts, ext)
			}
		}
	}

	// Parse filter codes
	var filtCodes []int
	if *filterCodes != "" {
		for _, c := range strings.Split(*filterCodes, ",") {
			if code, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
				filtCodes = append(filtCodes, code)
			}
		}
	}

	// Parse filter sizes
	var filtSizes []int64
	if *filterSize != "" {
		for _, s := range strings.Split(*filterSize, ",") {
			if size, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				filtSizes = append(filtSizes, size)
			}
		}
	}

	// Load wordlist
	wlManager, err := wordlist.NewManager(*wordlistPath)
	if err != nil {
		utils.PrintError("%s", err)
		os.Exit(1)
	}

	words, err := wlManager.Load()
	if err != nil {
		utils.PrintError("%s", err)
		os.Exit(1)
	}

	// Output writer
	writer, err := output.NewWriter(*outputFile)
	if err != nil {
		utils.PrintError("%s", err)
		os.Exit(1)
	}
	defer writer.Close()

	// Config with optimal defaults
	config := &scanner.Config{
		TargetURL:    *targetURL,
		Words:        words,
		Threads:      *threads,
		Timeout:      10 * time.Second,
		UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Extensions:   exts,
		Recursive:    !*noRecursive, // Recursive ON by default
		MaxDepth:     *depth,
		AddSlash:     true, // Add slash ON by default
		FilterCodes:  filtCodes,
		ExcludeSizes: filtSizes,
	}

	engine := scanner.NewEngine(config, writer)

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println()
		utils.PrintWarning("Stopping...")
		engine.Stop()
	}()

	// Run
	if err := engine.Run(); err != nil {
		utils.PrintError("%s", err)
		os.Exit(1)
	}

	engine.PrintStats()
}

func printHelp() {
	utils.Banner()
	fmt.Println(`USAGE:
  xsearch -u <url> [options]

EXAMPLES:
  xsearch -u https://target.com                    # Full recursive scan
  xsearch -u https://target.com -x php,html        # With extensions
  xsearch -u https://target.com -o results.txt     # Save results
  xsearch -u https://target.com -nr                # Single scan (no recursion)
  xsearch -u https://target.com -fc 403,500        # Filter status codes

OPTIONS:
  -u <url>       Target URL (required)
  -w <file>      Custom wordlist
  -o <file>      Output file
  -t <n>         Threads (default: 50)
  -x <ext>       Extensions to test (comma-separated)
  -d <n>         Max recursion depth (default: 3)
  -nr            Disable recursive scanning
  -fc <codes>    Filter/hide status codes
  -fs <sizes>    Filter/hide by response size
  -q             Quiet mode
  -v             Version
  -h             Help

FEATURES (all enabled by default):
  • Recursive directory scanning (BFS)
  • Soft 404 detection (hash-based)
  • Directory detection with slash appending
  • URL deduplication
  • Real-time colored output`)
}
