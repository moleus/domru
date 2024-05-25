package authorizedhttp

import (
	myhttp "github.com/ad/domru/pkg/domru/http"
	"net/http"
)

type TokenProvider interface {
	GetToken() (string, error)
}

type TokenRefresher interface {
	RefreshToken() error
}

type Client struct {
	DefaultClient  myhttp.HTTPClient
	TokenProvider  TokenProvider
	TokenRefresher TokenRefresher
}

func NewClient(tokenProvider TokenProvider, tokenRefresher TokenRefresher) *Client {
	return &Client{
		TokenProvider:  tokenProvider,
		TokenRefresher: tokenRefresher,
	}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	token, err := c.TokenProvider.GetToken()
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Refresh the token
		err = c.TokenRefresher.RefreshToken()
		if err != nil {
			return nil, err
		}

		// Retry the request with the new token
		newToken, err := c.TokenProvider.GetToken()
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+newToken)
		resp, err = c.DefaultClient.Do(req)
	}

	return resp, err
}
