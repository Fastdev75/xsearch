package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Fastdev75/xsearch/internal/output"
	"github.com/Fastdev75/xsearch/internal/scanner"
	"github.com/Fastdev75/xsearch/internal/utils"
	"github.com/Fastdev75/xsearch/internal/wordlist"
)

const version = "1.0.4"
const repoOwner = "mcauet"
const repoName = "xsearch"

func main() {
	// Essential flags only
	targetURL := flag.String("u", "", "Target URL (required)")
	wordlistPath := flag.String("w", "", "Custom wordlist path")
	outputFile := flag.String("o", "", "Output file")
	threads := flag.Int("t", 100, "Threads (default: 100 for speed)")
	extensions := flag.String("x", "", "Extensions (e.g., php,html,js)")
	timeout := flag.Int("timeout", 5, "Timeout in seconds (default: 5)")

	// Simple toggles
	noRecursive := flag.Bool("nr", false, "Disable recursive mode")
	depth := flag.Int("d", 5, "Max recursion depth (default: 5)")

	// Filtering (advanced)
	filterCodes := flag.String("fc", "", "Filter status codes (e.g., 403,500)")
	filterSize := flag.String("fs", "", "Filter by size")

	// Display options
	silent := flag.Bool("q", false, "Quiet mode (no banner)")
	showVersion := flag.Bool("v", false, "Version")
	showHelp := flag.Bool("h", false, "Help")
	doUpgrade := flag.Bool("up", false, "Auto-upgrade to latest version")

	flag.Parse()

	if *showVersion {
		fmt.Printf("xsearch v%s - Fast Content Discovery\n", version)
		os.Exit(0)
	}

	if *doUpgrade {
		if err := selfUpgrade(); err != nil {
			utils.PrintError("Upgrade failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *showHelp || *targetURL == "" {
		printHelp()
		os.Exit(0)
	}

	if !*silent {
		utils.Banner()
	}

	// Parse extensions - use defaults if not specified for complete discovery
	var exts []string
	if *extensions != "" {
		for _, ext := range strings.Split(*extensions, ",") {
			ext = strings.TrimSpace(strings.TrimPrefix(ext, "."))
			if ext != "" {
				exts = append(exts, ext)
			}
		}
	} else {
		// Default extensions - optimized for speed and common findings
		exts = []string{
			"php", "html", "js", "txt", "xml", "json",
			"bak", "old", "sql", "log", "env", "config",
			"asp", "aspx", "jsp", "zip", "gz",
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

	// Config with optimized defaults for speed
	config := &scanner.Config{
		TargetURL:    *targetURL,
		Words:        words,
		Threads:      *threads,
		Timeout:      time.Duration(*timeout) * time.Second,
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
  xsearch -u https://target.com                    # FULL content discovery (dirs + files)
  xsearch -u https://target.com -x php,html        # Custom extensions only
  xsearch -u https://target.com -o results.txt     # Save results
  xsearch -u https://target.com -t 200             # High-speed scan (200 threads)
  xsearch -u https://target.com -nr                # Single depth (no recursion)
  xsearch -u https://target.com -fc 403,500        # Filter status codes
  xsearch -u https://target.com -d 10              # Deep recursion (10 levels)

OPTIONS:
  -u <url>       Target URL (required)
  -w <file>      Custom wordlist
  -o <file>      Output file
  -t <n>         Threads (default: 100)
  -x <ext>       Extensions (default: php,html,js,txt,json,xml,bak,sql,etc.)
  -d <n>         Max recursion depth (default: 5)
  -timeout <s>   HTTP timeout in seconds (default: 5)
  -nr            Disable recursive scanning
  -fc <codes>    Filter/hide status codes
  -fs <sizes>    Filter/hide by response size
  -q             Quiet mode
  -v             Version
  -h             Help

SCAN STRATEGY (3 phases - all automatic):
  1. Directory Discovery - Fast HEAD requests to find directories
  2. Recursive Scan     - Subdirectories at each depth level
  3. File Discovery     - Find files (30+ extensions by default)

DEFAULT EXTENSIONS (17):
  php html js txt xml json bak old sql log env config
  asp aspx jsp zip gz

OPTIMIZATIONS:
  - HEAD requests for fast discovery
  - GET only for verification
  - Multi-point soft 404 calibration
  - High concurrency (100 threads)
  - HTTP/2 + connection pooling

UPGRADE:
  -up            Auto-upgrade to latest version from GitHub`)
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// selfUpgrade downloads and installs the latest version from GitHub
func selfUpgrade() error {
	utils.PrintInfo("Checking for updates...")

	// Get latest release info
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if latestVersion == currentVersion {
		utils.PrintSuccess("Already running the latest version (v%s)", currentVersion)
		return nil
	}

	utils.PrintInfo("New version available: v%s (current: v%s)", latestVersion, currentVersion)

	// Find the right asset for this OS/arch
	osName := runtime.GOOS
	archName := runtime.GOARCH
	assetName := fmt.Sprintf("xsearch_%s_%s", osName, archName)
	if osName == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, osName) && strings.Contains(asset.Name, archName) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		// Fallback: try to install via go install
		utils.PrintWarning("No pre-built binary found, trying go install...")
		cmd := exec.Command("go", "install", fmt.Sprintf("github.com/%s/%s/cmd/xsearch@%s", repoOwner, repoName, release.TagName))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go install failed: %w", err)
		}
		utils.PrintSuccess("Upgraded to v%s via go install", latestVersion)
		return nil
	}

	// Download the binary
	utils.PrintInfo("Downloading %s...", assetName)
	resp, err = client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "xsearch-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Copy download to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Replace current executable
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Try copy if rename fails (cross-device)
		src, _ := os.Open(tmpPath)
		dst, err := os.OpenFile(execPath, os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			src.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to update binary: %w", err)
		}
		io.Copy(dst, src)
		src.Close()
		dst.Close()
		os.Remove(tmpPath)
	}

	utils.PrintSuccess("Upgraded to v%s", latestVersion)
	return nil
}
