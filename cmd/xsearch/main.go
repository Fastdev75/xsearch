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

	"github.com/mcauet/xsearch/internal/output"
	"github.com/mcauet/xsearch/internal/scanner"
	"github.com/mcauet/xsearch/internal/utils"
	"github.com/mcauet/xsearch/internal/wordlist"
)

const version = "1.0.0"

func main() {
	// Define flags
	targetURL := flag.String("u", "", "Target URL (required)")
	wordlistPath := flag.String("w", "", "Wordlist path (default: SecLists common.txt)")
	outputFile := flag.String("o", "", "Output file for valid URLs")
	threads := flag.Int("t", 50, "Number of concurrent threads")
	timeout := flag.Int("timeout", 10, "HTTP request timeout in seconds")
	statusCodesStr := flag.String("status", "", "Filter by status codes (comma-separated, e.g., 200,301,403)")
	extensions := flag.String("x", "", "File extensions to check (comma-separated, e.g., php,html,js)")
	userAgent := flag.String("ua", "Xsearch/1.0", "Custom User-Agent")
	silent := flag.Bool("silent", false, "Disable banner")
	showVersion := flag.Bool("version", false, "Show version")
	showHelp := flag.Bool("h", false, "Show help")

	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("Xsearch v%s\n", version)
		os.Exit(0)
	}

	// Show help
	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Show banner
	if !*silent {
		utils.Banner()
	}

	// Validate required flags
	if *targetURL == "" {
		utils.PrintError("Target URL is required. Use -u <url>")
		fmt.Println()
		printUsage()
		os.Exit(1)
	}

	// Parse status codes
	var statusCodes []int
	if *statusCodesStr != "" {
		for _, code := range strings.Split(*statusCodesStr, ",") {
			code = strings.TrimSpace(code)
			if c, err := strconv.Atoi(code); err == nil {
				statusCodes = append(statusCodes, c)
			}
		}
	}

	// Parse extensions
	var exts []string
	if *extensions != "" {
		for _, ext := range strings.Split(*extensions, ",") {
			ext = strings.TrimSpace(ext)
			if ext != "" {
				exts = append(exts, ext)
			}
		}
	}

	// Initialize wordlist manager
	wlManager, err := wordlist.NewManager(*wordlistPath)
	if err != nil {
		utils.PrintError("%s", err)
		os.Exit(1)
	}

	// Load wordlist
	words, err := wlManager.Load()
	if err != nil {
		utils.PrintError("Failed to load wordlist: %s", err)
		os.Exit(1)
	}

	// Initialize output writer
	writer, err := output.NewWriter(*outputFile)
	if err != nil {
		utils.PrintError("Failed to create output file: %s", err)
		os.Exit(1)
	}
	defer writer.Close()

	// Create scanner config
	config := &scanner.Config{
		TargetURL:   *targetURL,
		Words:       words,
		Threads:     *threads,
		Timeout:     time.Duration(*timeout) * time.Second,
		UserAgent:   *userAgent,
		StatusCodes: statusCodes,
		Extensions:  exts,
	}

	// Create and run scanner
	engine := scanner.NewEngine(config, writer)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println()
		utils.PrintWarning("Interrupt received, stopping scan...")
		engine.Stop()
	}()

	// Run scan
	if err := engine.Run(); err != nil {
		utils.PrintError("Scan failed: %s", err)
		os.Exit(1)
	}

	// Print final statistics
	engine.PrintStats()
}

func printHelp() {
	utils.Banner()
	fmt.Println("Xsearch - Modern Web Content Discovery Tool")
	fmt.Println()
	printUsage()
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -u string        Target URL (required)")
	fmt.Println("  -w string        Wordlist path (default: SecLists common.txt)")
	fmt.Println("  -o string        Output file for valid URLs")
	fmt.Println("  -t int           Number of concurrent threads (default: 50)")
	fmt.Println("  -timeout int     HTTP request timeout in seconds (default: 10)")
	fmt.Println("  -status string   Filter by status codes (comma-separated)")
	fmt.Println("  -x string        File extensions to check (comma-separated)")
	fmt.Println("  -ua string       Custom User-Agent (default: Xsearch/1.0)")
	fmt.Println("  -silent          Disable banner")
	fmt.Println("  -version         Show version")
	fmt.Println("  -h               Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  xsearch -u https://target.com")
	fmt.Println("  xsearch -u https://target.com -w wordlist.txt -o results.txt")
	fmt.Println("  xsearch -u https://target.com -t 100 -x php,html,js")
	fmt.Println("  xsearch -u https://target.com -status 200,301,403")
	fmt.Println()
	fmt.Println("Note: Default wordlist requires SecLists to be installed:")
	fmt.Println("  sudo apt install seclists")
}

func printUsage() {
	fmt.Println("Usage: xsearch -u <url> [options]")
}
