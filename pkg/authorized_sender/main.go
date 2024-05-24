package authorized_sender

import (
	"fmt"
	"github.com/ad/domru/pkg/domru"
	"github.com/ad/domru/pkg/token_provider"
	"net/http"
)

type AuthorizedClient struct {
	client        *http.Client
	tokenProvider token_provider.TokenProvider
}

func NewAuthorizedClient(tokenProvider token_provider.TokenProvider, options ...func(client *AuthorizedClient)) *AuthorizedClient {
	authClient := &AuthorizedClient{
		tokenProvider: tokenProvider,
	}
	for _, option := range options {
		option(authClient)
	}
	return authClient
}

func WithClient(client *http.Client) func(*AuthorizedClient) {
	return func(c *AuthorizedClient) {
		c.client = client
	}
}

func (s *AuthorizedClient) Send(path, method string, output interface{}) error {
	token, err := s.tokenProvider.GetToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s", domru.BaseUrl, path)
	return domru.NewUpstreamRequest(url,
		domru.WithTokenString(token),
		domru.WithClient(s.client),
	).Send(method, output)
}
