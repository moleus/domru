package token_provider

import (
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/domru/helpers"
	"github.com/ad/domru/pkg/domru/models"
	"log"
	"net/http"
)

type TokenProvider interface {
	GetToken() (string, error)
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
	// this check slows down the application a lot
	//if !v.isTokenValid() {
	//	if err := v.RefreshToken(); err != nil {
	//		return "", fmt.Errorf("refresh expired token: %w", err)
	//	}
	//}

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

	var refreshTokenResponse models.AuthenticationResponse
	err = helpers.NewUpstreamRequest(constants.API_REFRESH_SESSION,
		helpers.WithHeader("Bearer", credentials.RefreshToken),
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

	err = helpers.NewUpstreamRequest(v.checkTokenUrl, helpers.WithTokenString(credentials.AccessToken)).Send(http.MethodGet, nil)
	if err != nil {
		log.Printf("error while checking token: %v", err)
		return false
	}

	return true
}
