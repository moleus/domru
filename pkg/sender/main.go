package sender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var defaultHeaders = map[string]string{
	"Content-Type":    "application/json",
	"Accept":          "application/json",
	"User-Agent":      "Android 12.0",
	"Connection":      "Keep-Alive",
	"Accept-Encoding": "gzip, deflate",
}

type UpstreamSender struct {
	url     string
	body    interface{}
	headers http.Header
}

func NewUpstreamSender(url string, options ...func(sender *UpstreamSender)) *UpstreamSender {
	headers := make(http.Header)
	for key, value := range defaultHeaders {
		headers.Add(key, value)
	}
	sender := &UpstreamSender{url: url, headers: headers, body: nil}

	for _, option := range options {
		option(sender)
	}
	return sender
}

func WithHeader(key string, value string) func(*UpstreamSender) {
	return func(u *UpstreamSender) {
		u.headers.Add(key, value)
	}
}

func WithBody(body interface{}) func(*UpstreamSender) {
	return func(u *UpstreamSender) {
		u.body = body
	}
}

func (s *UpstreamSender) Send(method string, output interface{}) error {
	var requestBody io.Reader = nil

	if s.body != nil {
		jsonBody, err := json.Marshal(s.body)
		if err != nil {
			return fmt.Errorf("failed to marshal body: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, s.url, requestBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = s.headers

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var content []byte
		if content, err = io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("failed to read response content: %w. Status code: %d", err, resp.StatusCode)
		}
		return fmt.Errorf("unexpected status code: %d. With content: %s", resp.StatusCode, string(content))
	}

	if output == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
