package scanner

// Job represents a scanning job
type Job struct {
	URL   string
	Depth int
}

// Result represents a scan result
type Result struct {
	URL        string
	StatusCode int
	Size       int64
	BodyHash   string
	Depth      int
	Error      error
}
