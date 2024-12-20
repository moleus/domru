package authorizedhttp

import (
	"log/slog"
	"net/http"
	"strconv"

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

	loginURL string
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
	resp, err := c.tryRequest(req)
	if err != nil {
		c.Logger.With("error", err).With("url", req.URL).With("method", req.Method).With("headers", req.Header).Warn("Failed to send request")
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Refresh the token
		c.Logger.Debug("Token expired. Refreshing token...")
		err = c.tokenRefresher.RefreshToken()
		if err != nil {
			c.Logger.With("err", err).Warn("Failed to refresh token. Redirecting to login page")
			return nil, NewTokenRefreshError(err)
		}

		return c.tryRequest(req)
	}

	return resp, err
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
		c.Logger.With("error", err).With("url", req.URL).With("method", req.Method).With("headers", req.Header).Warn("Failed to send request")
		return nil, err
	}
	return resp, nil
}
