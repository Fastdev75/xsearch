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
	threads := flag.Int("t", 150, "Threads (default: 150)")
	extensions := flag.String("x", "", "Extensions (e.g., php,html,js)")
	timeout := flag.Int("timeout", 10, "Timeout in seconds (default: 10)")

	// Simple toggles
	noRecursive := flag.Bool("nr", false, "Disable recursive mode")
	depth := flag.Int("d", 10, "Max recursion depth (default: 10)")

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
		// Default extensions - comprehensive web content discovery
		exts = []string{
			// Web scripts
			"php", "php3", "php4", "php5", "phtml", "inc",
			"asp", "aspx", "jsp", "jspx", "do", "action",
			"html", "htm", "xhtml", "shtml",
			"js", "ts", "jsx", "tsx", "vue", "mjs",
			// Data & Config
			"json", "xml", "yaml", "yml", "toml", "ini", "conf", "config", "cfg",
			"env", "properties", "htaccess", "htpasswd",
			// Backup & Source
			"bak", "backup", "old", "orig", "copy", "tmp", "temp", "swp",
			"sql", "db", "sqlite", "mdb",
			"log", "logs", "txt", "md", "csv",
			// Archives
			"zip", "tar", "gz", "rar", "7z", "tgz",
			// Special
			"git", "svn", "DS_Store",
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
  xsearch -u https://target.com                    # FULL aggressive discovery
  xsearch -u https://target.com -o results.txt     # Save results to file
  xsearch -u https://target.com -t 300             # Ultra-fast (300 threads)
  xsearch -u https://target.com -x php,html        # Custom extensions only
  xsearch -u https://target.com -nr                # No recursion (fast scan)
  xsearch -u https://target.com -fc 403            # Hide 403 responses

OPTIONS:
  -u <url>       Target URL (required)
  -w <file>      Custom wordlist (auto-downloads if none)
  -o <file>      Output file (URLs only, deduplicated)
  -t <n>         Threads (default: 150)
  -x <ext>       Extensions (default: 50+ extensions)
  -d <n>         Max recursion depth (default: 10)
  -timeout <s>   Timeout in seconds (default: 10)
  -nr            Disable recursive scanning
  -fc <codes>    Filter status codes (e.g., 403,500)
  -fs <sizes>    Filter by size (e.g., 0,1234)
  -q             Quiet mode (no banner)
  -v             Version
  -h             Help
  -up            Auto-upgrade from GitHub

DEFAULT BEHAVIOR (no flags needed):
  - 150 concurrent threads
  - 10 levels deep recursion
  - 50+ file extensions tested
  - Auto soft-404 detection
  - Directories + files discovery

EXTENSIONS (50+):
  Scripts:  php php3-5 asp aspx jsp html js ts vue
  Config:   json xml yaml env ini conf htaccess
  Backup:   bak old sql log zip tar gz
  Special:  git svn DS_Store

OPTIMIZATIONS:
  - HEAD requests for speed
  - Dynamic soft-404 filtering
  - Connection pooling + HTTP/2
  - Real-time progress bar`)
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
