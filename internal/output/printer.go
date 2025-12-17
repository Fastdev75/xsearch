package output

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Fastdev75/xsearch/internal/utils"
)

// Printer handles real-time terminal output with tree structure
type Printer struct {
	mu           sync.Mutex
	statusFilter map[int]bool
	showAll      bool
}

// NewPrinter creates a new output printer
func NewPrinter(statusCodes []int) *Printer {
	p := &Printer{
		statusFilter: make(map[int]bool),
	}

	if len(statusCodes) == 0 {
		p.showAll = true
	} else {
		for _, code := range statusCodes {
			p.statusFilter[code] = true
		}
	}

	return p
}

// PrintResult prints a scan result with hierarchical tree structure
func (p *Printer) PrintResult(url string, statusCode int, size int64, isDir bool, depth int) bool {
	if !p.showAll && !p.statusFilter[statusCode] {
		return false
	}

	if p.showAll && statusCode == 404 {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	color := p.getStatusColor(statusCode)
	sizeStr := formatSize(size)

	// Type indicator with icon
	var typeIcon, typeColor string
	if isDir {
		typeIcon = "📁"
		typeColor = utils.Cyan
	} else {
		typeIcon = "📄"
		typeColor = utils.White
	}

	// Build tree prefix based on depth
	var prefix string
	if depth == 0 {
		// Root level - no prefix
		prefix = ""
	} else if depth == 1 {
		// First level subdirectory
		prefix = "├── "
	} else {
		// Deeper levels with visual hierarchy
		prefix = strings.Repeat("│   ", depth-1) + "├── "
	}

	// Format: prefix [STATUS] 📁/📄 URL [SIZE]
	fmt.Printf("%s%s[%d]%s %s%s%s %s %s[%s]%s\n",
		prefix,
		color, statusCode, utils.Reset,
		typeColor, typeIcon, utils.Reset,
		url,
		utils.White, sizeStr, utils.Reset)

	return true
}

// getStatusColor returns the appropriate color for a status code
func (p *Printer) getStatusColor(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return utils.Green
	case statusCode >= 300 && statusCode < 400:
		return utils.Blue
	case statusCode >= 400 && statusCode < 500:
		return utils.Yellow
	case statusCode >= 500:
		return utils.Red
	default:
		return utils.White
	}
}

// formatSize formats the content size
func formatSize(size int64) string {
	if size < 0 {
		return "N/A"
	}
	if size == 0 {
		return "0B"
	}
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
}

// ShouldShow checks if a status code should be displayed
func (p *Printer) ShouldShow(statusCode int) bool {
	if p.showAll {
		return statusCode != 404
	}
	return p.statusFilter[statusCode]
}

// IsInteresting checks if a status code is considered interesting for output file
func IsInteresting(statusCode int) bool {
	interesting := map[int]bool{
		200: true, 201: true, 204: true,
		301: true, 302: true, 307: true, 308: true,
		401: true, 403: true, 405: true,
	}
	return interesting[statusCode]
}
