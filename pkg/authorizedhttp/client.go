package authorizedhttp

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	myhttp "github.com/moleus/domru/pkg/domru/http"
)

type TokenRefreshError struct {
	Err error
}

func (e TokenRefreshError) Error() string {
	return e.Err.Error()
}

func NewTokenRefreshError(err error) TokenRefreshError {
	return TokenRefreshError{Err: err}
}

type TokenProvider interface {
	GetToken() (string, error)
}

type OperatorProvider interface {
	GetOperatorID() (int, error)
}

type TokenRefresher interface {
	RefreshToken() error
}

type Client struct {
	DefaultClient  myhttp.HTTPClient
	tokenProvider  TokenProvider
	tokenRefresher TokenRefresher
	Logger         *slog.Logger

	operatorProvider OperatorProvider

	loginURL  string
	refreshMu sync.Mutex
}

func NewClient(tokenProvider TokenProvider, tokenRefresher TokenRefresher, operatorProvider OperatorProvider) *Client {
	return &Client{
		tokenProvider:    tokenProvider,
		tokenRefresher:   tokenRefresher,
		operatorProvider: operatorProvider,
		DefaultClient:    http.DefaultClient,
		Logger:           slog.Default(),
		loginURL:         "/pages/login.html",
	}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// Incoming proxy requests have no GetBody. Buffer only bounded request
	// bodies so a confirmed 401 can be retried with the original POST payload.
	r := req.Clone(req.Context())
	if r.Body != nil && r.Body != http.NoBody && r.GetBody == nil {
		data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		r.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 1<<20 {
			return nil, fmt.Errorf("request body exceeds 1 MiB")
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }
	}
	resp, err := c.tryRequest(r)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	resp.Body.Close()
	used := r.Header.Get("Authorization")
	c.refreshMu.Lock()
	current, tokenErr := c.tokenProvider.GetToken()
	if tokenErr == nil && used == "Bearer "+current {
		tokenErr = c.tokenRefresher.RefreshToken()
	}
	c.refreshMu.Unlock()
	if tokenErr != nil {
		return nil, NewTokenRefreshError(tokenErr)
	}
	if r.GetBody != nil {
		r.Body, err = r.GetBody()
		if err != nil {
			return nil, err
		}
	}
	return c.tryRequest(r)
}

func (c *Client) tryRequest(req *http.Request) (*http.Response, error) {
	newToken, err := c.tokenProvider.GetToken()
	if err != nil {
		c.Logger.With("error", err).Warn("Failed to get new token")
		return nil, err
	}

	operatorID, err := c.operatorProvider.GetOperatorID()
	if err != nil {
		c.Logger.With("error", err).Warn("Failed to get operator id")
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+newToken)
	req.Header.Set("Operator", strconv.Itoa(operatorID))
	resp, err := c.DefaultClient.Do(req)
	if err != nil {
		c.Logger.Warn("Authorized HTTP request failed")
		return nil, err
	}
	return resp, nil
}
