package scanner

import (
	"net/http"
	"sync"

	"github.com/mcauet/xsearch/internal/httpclient"
)

// Job represents a scanning job
type Job struct {
	URL  string
	Path string
}

// Result represents a scan result
type Result struct {
	URL        string
	StatusCode int
	Size       int64
	Error      error
}

// Worker handles HTTP requests
type Worker struct {
	id        int
	client    *http.Client
	userAgent string
	jobs      <-chan Job
	results   chan<- Result
	wg        *sync.WaitGroup
}

// NewWorker creates a new worker
func NewWorker(id int, client *http.Client, userAgent string, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) *Worker {
	return &Worker{
		id:        id,
		client:    client,
		userAgent: userAgent,
		jobs:      jobs,
		results:   results,
		wg:        wg,
	}
}

// Start begins processing jobs
func (w *Worker) Start() {
	defer w.wg.Done()

	for job := range w.jobs {
		result := w.processJob(job)
		w.results <- result
	}
}

// processJob performs the HTTP request for a job
func (w *Worker) processJob(job Job) Result {
	httpResult := httpclient.Request(w.client, job.URL, w.userAgent)

	return Result{
		URL:        httpResult.URL,
		StatusCode: httpResult.StatusCode,
		Size:       httpResult.Size,
		Error:      httpResult.Error,
	}
}
