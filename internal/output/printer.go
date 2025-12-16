package output

import (
	"fmt"
	"strings"
	"sync"

	"xsearch/internal/utils"
)

// Printer handles real-time terminal output
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
		// Default: show all interesting status codes
		p.showAll = true
	} else {
		for _, code := range statusCodes {
			p.statusFilter[code] = true
		}
	}

	return p
}

// PrintResult prints a scan result to the terminal with color coding
func (p *Printer) PrintResult(url string, statusCode int, size int64, isDir bool, depth int) bool {
	// Check if we should display this status code
	if !p.showAll && !p.statusFilter[statusCode] {
		return false
	}

	// Skip 404 by default unless explicitly requested
	if p.showAll && statusCode == 404 {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	color := p.getStatusColor(statusCode)
	sizeStr := formatSize(size)

	// Create indentation based on depth
	indent := strings.Repeat("  ", depth)

	// Type indicator
	typeIcon := "FILE"
	typeColor := utils.White
	if isDir {
		typeIcon = "DIR "
		typeColor = utils.Cyan
	}

	fmt.Printf("%s%s[%d]%s %s%s%s %s %s[%s]%s\n",
		indent,
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
		200: true,
		201: true,
		204: true,
		301: true,
		302: true,
		307: true,
		308: true,
		401: true,
		403: true,
		405: true,
		500: true,
		501: true,
		502: true,
		503: true,
	}
	return interesting[statusCode]
}
