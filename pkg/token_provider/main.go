package token_provider

import (
	"fmt"
	"github.com/ad/domru/handlers"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/sender"
	"log"
	"net/http"
)

type TokenProvider interface {
	GetToken() string
}

type ValidTokenProvider struct {
	credentialsStore auth.CredentialsStore
	checkTokenUrl    string
}

func NewValidTokenProvider(credentialsStore auth.CredentialsStore, checkTokenUrl string) *ValidTokenProvider {
	return &ValidTokenProvider{
		credentialsStore: credentialsStore,
		checkTokenUrl:    checkTokenUrl,
	}
}

func (v *ValidTokenProvider) GetToken() (string, error) {
	if !v.isTokenValid() {
		if err := v.RefreshToken(); err != nil {
			return "", fmt.Errorf("refresh expired token: %w", err)
		}
	}

	credentials, err := v.credentialsStore.LoadCredentials()
	if err != nil {
		return "", fmt.Errorf("load credentials: %w", err)
	}

	return credentials.AccessToken, nil
}

func (v *ValidTokenProvider) RefreshToken() error {
	credentials, err := v.credentialsStore.LoadCredentials()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	var refreshTokenResponse auth.AuthenticationResponse
	err = sender.NewUpstreamSender(handlers.API_REFRESH_SESSION,
		sender.WithHeader("Bearer", credentials.RefreshToken),
	).Send(http.MethodGet, &refreshTokenResponse)
	if err != nil {
		return fmt.Errorf("send request to refresh token: %w", err)
	}

	err = v.credentialsStore.SaveCredentials(refreshTokenResponse)
	if err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	return nil
}

func (v *ValidTokenProvider) isTokenValid() bool {
	credentials, err := v.credentialsStore.LoadCredentials()
	if err != nil {
		return false
	}

	err = sender.NewUpstreamSender(v.checkTokenUrl, sender.WithHeader("Authorization", "Bearer "+credentials.AccessToken)).Send(http.MethodGet, nil)
	if err != nil {
		log.Printf("error while checking token: %v", err)
		return false
	}

	return true
}
