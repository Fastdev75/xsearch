package wordlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Fastdev75/xsearch/internal/utils"
)

// Default wordlist paths (in order of preference)
var DefaultWordlists = []string{
	"/usr/share/seclists/Discovery/Web-Content/DirBuster-2007_directory-list-2.3-medium.txt",
	"/usr/share/seclists/Discovery/Web-Content/big.txt",
	"/usr/share/seclists/Discovery/Web-Content/common.txt",
	"/usr/share/wordlists/dirb/big.txt",
	"/usr/share/wordlists/dirb/common.txt",
}

// Manager handles wordlist operations
type Manager struct {
	path  string
	words []string
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
		if !found {
			return nil, fmt.Errorf("no wordlist found. Install SecLists: sudo apt install seclists")
		}
	}

	// Verify wordlist file exists
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil, fmt.Errorf("wordlist not found: %s", m.path)
	}

	return m, nil
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
