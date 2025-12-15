package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Config holds HTTP client configuration
type Config struct {
	Timeout         time.Duration
	FollowRedirects bool
	UserAgent       string
}

// DefaultConfig returns a default HTTP client configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:         10 * time.Second,
		FollowRedirects: false,
		UserAgent:       "Xsearch/1.0",
	}
}

// NewClient creates a new optimized HTTP client
func NewClient(cfg *Config) *http.Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	// Disable redirect following if configured
	if !cfg.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}

// Result holds the HTTP request result
type Result struct {
	URL        string
	StatusCode int
	Size       int64
	Error      error
}

// Request performs an HTTP GET request and returns the result
func Request(client *http.Client, url string, userAgent string) *Result {
	result := &Result{URL: url}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = err
		return result
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Size = resp.ContentLength

	return result
}
