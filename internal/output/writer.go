package output

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

// Writer handles file output for valid URLs
type Writer struct {
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	enabled  bool
	filePath string
	format   string // "url", "json", or "full"
}

// NewWriter creates a new file writer
func NewWriter(outputPath string) (*Writer, error) {
	w := &Writer{
		filePath: outputPath,
		enabled:  outputPath != "",
		format:   "full",
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

// WriteURL writes a valid URL to the output file (URL only, no status code)
func (w *Writer) WriteURL(url string) error {
	if !w.enabled {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.writer.WriteString(url + "\n")
	if err != nil {
		return err
	}

	// Flush immediately for real-time output
	return w.writer.Flush()
}

// WriteResult writes a full result line to the output file
func (w *Writer) WriteResult(url string, statusCode int, size int64, isDir bool) error {
	if !w.enabled {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	typeStr := "FILE"
	if isDir {
		typeStr = "DIR"
	}

	line := fmt.Sprintf("[%d] [%s] [%d] %s\n", statusCode, typeStr, size, url)
	_, err := w.writer.WriteString(line)
	if err != nil {
		return err
	}

	// Flush immediately for real-time output
	return w.writer.Flush()
}

// Close closes the file writer
func (w *Writer) Close() error {
	if !w.enabled || w.file == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return err
	}

	return w.file.Close()
}

// IsEnabled returns whether file output is enabled
func (w *Writer) IsEnabled() bool {
	return w.enabled
}

// GetPath returns the output file path
func (w *Writer) GetPath() string {
	return w.filePath
}
