package wordlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mcauet/xsearch/internal/utils"
)

const (
	// DefaultSecListsPath is the default path for SecLists on Kali Linux
	DefaultSecListsPath = "/usr/share/seclists"
	// DefaultWordlist is the default wordlist path
	DefaultWordlist = "/usr/share/seclists/Discovery/Web-Content/common.txt"
)

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
		// Check if SecLists is installed
		if !m.checkSecLists() {
			return nil, fmt.Errorf("SecLists not found. Please install it:\n  sudo apt install seclists -y")
		}
		m.path = DefaultWordlist
	}

	// Verify wordlist file exists
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return nil, fmt.Errorf("wordlist not found: %s", m.path)
	}

	return m, nil
}

// checkSecLists checks if SecLists is installed
func (m *Manager) checkSecLists() bool {
	_, err := os.Stat(DefaultSecListsPath)
	return err == nil
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
	utils.PrintInfo("Loaded %d words from %s", len(words), m.path)

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
