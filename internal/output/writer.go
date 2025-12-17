package output

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"sync"
)

// Writer handles file output for valid URLs with hierarchical structure
type Writer struct {
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	enabled  bool
	filePath string
	urls     []string // Collect URLs for sorted output
}

// NewWriter creates a new file writer
func NewWriter(outputPath string) (*Writer, error) {
	w := &Writer{
		filePath: outputPath,
		enabled:  outputPath != "",
		urls:     make([]string, 0, 100),
	}

	if !w.enabled {
		return w, nil
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}

	w.file = file
	w.writer = bufio.NewWriter(file)

	return w, nil
}

// WriteURL collects URL for final sorted output
func (w *Writer) WriteURL(url string) error {
	if !w.enabled {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.urls = append(w.urls, url)
	return nil
}

// WriteResult writes a full result line (legacy, not used)
func (w *Writer) WriteResult(url string, statusCode int, size int64, isDir bool) error {
	return w.WriteURL(url)
}

// Close writes sorted hierarchical output and closes the file
func (w *Writer) Close() error {
	if !w.enabled || w.file == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Sort URLs for hierarchical display
	sort.Strings(w.urls)

	// Group URLs by base path for tree structure
	tree := buildTree(w.urls)

	// Write tree
	writeTree(w.writer, tree, "")

	if err := w.writer.Flush(); err != nil {
		return err
	}

	return w.file.Close()
}

// TreeNode represents a node in the URL tree
type TreeNode struct {
	name     string
	fullURL  string
	children map[string]*TreeNode
	isLeaf   bool
}

// buildTree constructs a tree from URLs
func buildTree(urls []string) *TreeNode {
	root := &TreeNode{
		name:     "",
		children: make(map[string]*TreeNode),
	}

	for _, url := range urls {
		// Parse URL into parts
		parts := parseURLParts(url)
		current := root

		for i, part := range parts {
			if current.children == nil {
				current.children = make(map[string]*TreeNode)
			}

			if _, exists := current.children[part]; !exists {
				current.children[part] = &TreeNode{
					name:     part,
					children: make(map[string]*TreeNode),
				}
			}

			current = current.children[part]

			// Mark the last part as having the full URL
			if i == len(parts)-1 {
				current.fullURL = url
				current.isLeaf = true
			}
		}
	}

	return root
}

// parseURLParts extracts path parts from URL
func parseURLParts(url string) []string {
	// Remove protocol
	path := url
	if idx := strings.Index(url, "://"); idx != -1 {
		path = url[idx+3:]
	}

	// Split by /
	parts := strings.Split(path, "/")

	// Filter empty parts but keep meaningful ones
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}

// writeTree writes the tree structure to writer
func writeTree(w *bufio.Writer, node *TreeNode, prefix string) {
	if node == nil {
		return
	}

	// Get sorted children keys
	var keys []string
	for k := range node.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, key := range keys {
		child := node.children[key]
		isLast := i == len(keys)-1

		// Determine connector
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		// Write the URL if it's a leaf or has a fullURL
		if child.fullURL != "" {
			w.WriteString(prefix + connector + child.fullURL + "\n")
		}

		// Recurse into children
		newPrefix := prefix
		if isLast {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}

		if len(child.children) > 0 && child.fullURL != "" {
			// Has children, recurse
			writeTree(w, child, newPrefix)
		} else if len(child.children) > 0 {
			// Intermediate node without URL, recurse without printing
			writeTree(w, child, prefix)
		}
	}
}

// IsEnabled returns whether file output is enabled
func (w *Writer) IsEnabled() bool {
	return w.enabled
}

// GetPath returns the output file path
func (w *Writer) GetPath() string {
	return w.filePath
}

// GetCount returns the number of URLs collected
func (w *Writer) GetCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.urls)
}
