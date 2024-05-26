package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ad/domru/pkg/domru/constants"
	myhttp "github.com/ad/domru/pkg/domru/http"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

var defaultHeaders = map[string]string{
	"Content-Type":    "application/json",
	"Accept":          "application/json",
	"User-Agent":      constants.CLIENT_USERAGENT,
	"Connection":      "Keep-Alive",
	"Accept-Encoding": "gzip, deflate",
}

type UpstreamRequest struct {
	client  myhttp.HTTPClient
	url     string
	body    interface{}
	headers http.Header
}

func NewUpstreamRequest(url string, options ...func(sender *UpstreamRequest)) *UpstreamRequest {
	headers := make(http.Header)
	for key, value := range defaultHeaders {
		headers.Add(key, value)
	}
	sender := &UpstreamRequest{url: url, headers: headers, body: nil, client: http.DefaultClient}

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

func WithQueryParams(params url.Values) func(*UpstreamRequest) {
	return func(u *UpstreamRequest) {
		parsedUrl, err := url.Parse(u.url)
		if err != nil {
			log.Fatalf("failed to parse url %s: %v", u.url, err)
		}
		u.url = fmt.Sprintf("%s?%s", parsedUrl.String(), params.Encode())
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
		return fmt.Errorf("unexpected status code: %d. With content: %u", resp.StatusCode, string(content))
	}

	log.Printf("Request to %s took %s\n", u.url, time.Since(startTime))
	if output == nil {
		return nil
	}
	content, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read response content: %w. Status code: %d", readErr, resp.StatusCode)
	}

	if decodeErr := json.NewDecoder(bytes.NewReader(content)).Decode(&output); decodeErr != nil {
		return fmt.Errorf("failed to decode response. Body: '%s'. Error: %w. Response: %+v", content, decodeErr, resp)
	}
	return nil
}

func (u *UpstreamRequest) SendRequest(method string) (*http.Response, error) {
	var requestBody io.Reader = nil
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
	req.Header = u.headers

	resp, err := u.client.Do(req)
	return resp, err
}
