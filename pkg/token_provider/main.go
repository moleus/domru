package token_provider

import (
	"fmt"
	"github.com/ad/domru/handlers"
	"github.com/ad/domru/pkg/auth"
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
	var refreshTokenResponse auth.AuthenticationResponse
	err := auth.SendRequest(handlers.API_REFRESH_SESSION, http.MethodGet, nil, &refreshTokenResponse)
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
	resp, err := http.Get(v.checkTokenUrl)
	if err != nil {
		log.Printf("error while checking token: %v", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("unexpected status code: %d", resp.StatusCode)
		return false
	}

	return true
}
