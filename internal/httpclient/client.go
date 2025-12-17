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
		Timeout:         5 * time.Second, // Reduced from 10s for speed
		FollowRedirects: false,
		UserAgent:       "Xsearch/3.0",
	}
}

// NewClient creates a new highly optimized HTTP client
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
			KeepAlive: 60 * time.Second, // Increased for better connection reuse
		}).DialContext,
		// Optimized connection pool settings
		MaxIdleConns:          500,  // Increased from 200
		MaxIdleConnsPerHost:   200,  // Increased from 100
		MaxConnsPerHost:       200,  // Increased from 100
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second, // Reduced from 10s
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		DisableCompression:    true, // Disable for speed (we don't need to decompress)
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: cfg.Timeout,
		WriteBufferSize:       4096,  // Optimized buffer
		ReadBufferSize:        16384, // Optimized buffer for reading
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	// Disable redirect following for directory detection
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

	// Minimal headers for speed
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
	result.ContentType = resp.Header.Get("Content-Type")

	// Get redirect URL if applicable
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		result.RedirectURL = resp.Header.Get("Location")
	}

	if readBody {
		// Read body for accurate size and hash calculation
		// Limit to 512KB for speed (reduced from 1MB)
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
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
			body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
			if err == nil {
				result.Size = int64(len(body))
				result.BodyHash = fmt.Sprintf("%x", md5.Sum(body))
			}
		}
	}

	return result
}

// HeadRequest performs an HTTP HEAD request (much faster, no body transfer)
func HeadRequest(client *http.Client, url string, userAgent string) *Result {
	result := &Result{URL: url}

	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		result.Error = err
		return result
	}

	// Minimal headers for speed
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
