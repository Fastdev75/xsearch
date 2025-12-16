package httpclient

import (
	"crypto/md5"
	"crypto/tls"
	"fmt"
	"io"
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
		UserAgent:       "Xsearch/2.0",
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
			MinVersion:         tls.VersionTLS10,
		},
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
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
	URL         string
	StatusCode  int
	Size        int64
	BodyHash    string
	ContentType string
	RedirectURL string
	Error       error
}

// Request performs an HTTP GET request and returns the result (headers only)
func Request(client *http.Client, url string, userAgent string) *Result {
	return request(client, url, userAgent, false)
}

// RequestWithBody performs an HTTP GET request and reads the body for hashing
func RequestWithBody(client *http.Client, url string, userAgent string) *Result {
	return request(client, url, userAgent, true)
}

// request is the internal request function
func request(client *http.Client, url string, userAgent string, readBody bool) *Result {
	result := &Result{URL: url}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = err
		return result
	}

	// Set headers to mimic a real browser
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.ContentType = resp.Header.Get("Content-Type")

	// Get redirect URL if applicable
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		result.RedirectURL = resp.Header.Get("Location")
	}

	if readBody {
		// Read body for accurate size and hash calculation
		// Limit to 1MB to prevent memory issues
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if err != nil {
			result.Error = err
			return result
		}
		result.Size = int64(len(body))
		result.BodyHash = fmt.Sprintf("%x", md5.Sum(body))
	} else {
		// Just use Content-Length header
		result.Size = resp.ContentLength
		if result.Size < 0 {
			// Try to read a small portion to get actual size
			body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			if err == nil {
				result.Size = int64(len(body))
				result.BodyHash = fmt.Sprintf("%x", md5.Sum(body))
			}
		}
	}

	return result
}

// HeadRequest performs an HTTP HEAD request (faster, no body)
func HeadRequest(client *http.Client, url string, userAgent string) *Result {
	result := &Result{URL: url}

	req, err := http.NewRequest("HEAD", url, nil)
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
	result.ContentType = resp.Header.Get("Content-Type")

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		result.RedirectURL = resp.Header.Get("Location")
	}

	return result
}
