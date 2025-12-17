package wordlist

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fastdev75/xsearch/internal/utils"
)

// Default wordlist paths (in order of preference)
var DefaultWordlists = []string{
	"/usr/share/seclists/Discovery/Web-Content/directory-list-2.3-medium.txt",
	"/usr/share/seclists/Discovery/Web-Content/common.txt",
	"/usr/share/wordlists/dirb/common.txt",
}

// Bundled wordlist URL (SecLists common.txt)
const BundledWordlistURL = "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/common.txt"

// Manager handles wordlist operations
type Manager struct {
	path  string
	words []string
}

// getXsearchDir returns the xsearch data directory
func getXsearchDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/.xsearch"
	}
	return filepath.Join(home, ".xsearch")
}

// NewManager creates a new wordlist manager
func NewManager(customPath string) (*Manager, error) {
	m := &Manager{}

	if customPath != "" {
		m.path = customPath
	} else {
		// Find first available default wordlist
		found := false
		for _, wl := range DefaultWordlists {
			if _, err := os.Stat(wl); err == nil {
				m.path = wl
				found = true
				break
			}
		}

		// Check for bundled wordlist
		if !found {
			bundledPath := filepath.Join(getXsearchDir(), "wordlists", "common.txt")
			if _, err := os.Stat(bundledPath); err == nil {
				m.path = bundledPath
				found = true
			}
		}

		// Download wordlist if none found
		if !found {
			utils.PrintWarning("No wordlist found. Downloading default wordlist...")
			downloadedPath, err := downloadWordlist()
			if err != nil {
				return nil, fmt.Errorf("failed to download wordlist: %w\nInstall manually: sudo apt install seclists", err)
			}
			m.path = downloadedPath
			utils.PrintSuccess("Wordlist downloaded to: %s", downloadedPath)
		}
	}

	// Verify wordlist file exists
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil, fmt.Errorf("wordlist not found: %s", m.path)
	}

	return m, nil
}

// downloadWordlist downloads the default wordlist
func downloadWordlist() (string, error) {
	// Create directory
	wordlistDir := filepath.Join(getXsearchDir(), "wordlists")
	if err := os.MkdirAll(wordlistDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	destPath := filepath.Join(wordlistDir, "common.txt")

	// Download with timeout
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(BundledWordlistURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create file
	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy content
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return destPath, nil
}

// Load reads the wordlist file and returns words
func (m *Manager) Load() ([]string, error) {
	file, err := os.Open(m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open wordlist: %w", err)
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)

	// Increase buffer size for large lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" && !strings.HasPrefix(word, "#") {
			words = append(words, word)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading wordlist: %w", err)
	}

	m.words = words
	utils.PrintInfo("Wordlist: %s (%d entries)", m.path, len(words))

	return words, nil
}

// GetPath returns the wordlist path
func (m *Manager) GetPath() string {
	return m.path
}

// Count returns the number of words loaded
func (m *Manager) Count() int {
	return len(m.words)
}
