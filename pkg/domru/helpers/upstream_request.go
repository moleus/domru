package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	myhttp "github.com/moleus/domru/pkg/domru/http"
)

var defaultHeaders = map[string]string{
	"user-agent":      "Google sdkgphone64x8664 | Android 14 | erth | 8.9.2 (8090200) |  | null | 10c99d90-9899-4a25-926f-067b34bc4a7f | null",
	"content-type":    "application/json; charset=UTF-8",
	"connection":      "Keep-Alive",
	"accept-encoding": "gzip",
}

type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream error: %d, body: %s", e.StatusCode, e.Body)
}

func NewUpstreamError(statusCode int, body string) *UpstreamError {
	return &UpstreamError{StatusCode: statusCode, Body: body}
}

type UpstreamRequest struct {
	client  myhttp.HTTPClient
	url     string
	body    interface{}
	headers http.Header
	logger  *slog.Logger
}

func NewUpstreamRequest(url string, options ...func(sender *UpstreamRequest)) *UpstreamRequest {
	headers := make(http.Header)
	for key, value := range defaultHeaders {
		headers.Set(key, value)
	}
	sender := &UpstreamRequest{url: url, headers: headers, body: nil, client: http.DefaultClient, logger: slog.Default()}

	for _, option := range options {
		option(sender)
	}
	return sender
}

func WithClient(client myhttp.HTTPClient) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		u.client = client
	}
}

func WithHeader(key string, value string) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		u.headers.Add(key, value)
	}
}

func WithBody(body interface{}) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		u.body = body
	}
}

func WithLogger(logger *slog.Logger) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		u.logger = logger
	}
}

func WithQueryParams(params url.Values) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		parsedURL, err := url.Parse(u.url)
		if err != nil {
			log.Fatalf("failed to parse url %s: %v", u.url, err)
		}
		u.url = fmt.Sprintf("%s?%s", parsedURL.String(), params.Encode())
	}
}

func (u *UpstreamRequest) Send(method string, output interface{}) error {
	startTime := time.Now()

	resp, err := u.SendRequest(method)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var content []byte
		if content, err = io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("failed to read response content: %w. Status code: %d", err, resp.StatusCode)
		}
		u.logger.With("url", u.url).With("status", resp.StatusCode).With("request_headers", u.headers).With("request_body", u.body).With("response_body", string(content)).Debug("failed to send request")
		return NewUpstreamError(resp.StatusCode, string(content))
	}

	log.Printf("Request to %s took %s\n", u.url, time.Since(startTime))
	if output == nil {
		return nil
	}
	content, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		u.logger.With("url", u.url).With("status", resp.StatusCode).With("request_body", u.body).Debug("failed to read response content")
		return NewUpstreamError(resp.StatusCode, "")
	}

	if decodeErr := json.NewDecoder(bytes.NewReader(content)).Decode(&output); decodeErr != nil {
		u.logger.With("url", u.url).With("status", resp.StatusCode).With("request_body", u.body).Debug("failed to send request")
		return fmt.Errorf("decode response. First 100 characters of body: '%s'. Error: %w", content[:100], decodeErr)
	}
	return nil
}

func (u *UpstreamRequest) SendRequest(method string) (*http.Response, error) {
	var requestBody io.Reader
	if u.body != nil {
		jsonBody, err := json.Marshal(u.body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, u.url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for key, values := range u.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := u.client.Do(req)
	u.logger.With("url", req.URL).With("method", req.Method).With("headers", req.Header).Debug("Sent request")
	return resp, err
}
