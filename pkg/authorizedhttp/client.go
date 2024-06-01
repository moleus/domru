package authorizedhttp

import (
	myhttp "github.com/ad/domru/pkg/domru/http"
	"log/slog"
	"net/http"
	"strconv"
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
	GetOperatorId() (int, error)
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

	loginUrl   string
	operatorId int
}

func NewClient(tokenProvider TokenProvider, tokenRefresher TokenRefresher, operatorProvider OperatorProvider) *Client {
	return &Client{
		tokenProvider:    tokenProvider,
		tokenRefresher:   tokenRefresher,
		operatorProvider: operatorProvider,
		DefaultClient:    http.DefaultClient,
		Logger:           slog.Default(),
		loginUrl:         "/pages/login.html",
	}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	token, err := c.tokenProvider.GetToken()
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.DefaultClient.Do(req)
	if err != nil {
		c.Logger.With("error", err).With("url", req.URL).With("method", req.Method).With("headers", req.Header).Warn("Failed to send request")
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Refresh the token
		c.Logger.Debug("Token expired. Refreshing token...")
		err = c.tokenRefresher.RefreshToken()
		if err != nil {
			c.Logger.Warn("Failed to refresh token. Redirecting to login page", err.Error())
			return nil, NewTokenRefreshError(err)
		}

		// Retry the request with the new token
		newToken, err := c.tokenProvider.GetToken()
		if err != nil {
			c.Logger.With("error", err).Warn("Failed to get new token")
			return nil, err
		}

		operatorId, err := c.operatorProvider.GetOperatorId()
		if err != nil {
			c.Logger.With("error", err).Warn("Failed to get operator id")
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+newToken)
		req.Header.Set("Operator", strconv.Itoa(operatorId))
		resp, err = c.DefaultClient.Do(req)
	}

	return resp, err
}
