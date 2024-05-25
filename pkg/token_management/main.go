package token_management

import (
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/domru/helpers"
	"github.com/ad/domru/pkg/domru/models"
	"net/http"
)

type ValidTokenProvider struct {
	credentialsStore auth.CredentialsStore
}

func NewValidTokenProvider(credentialsStore auth.CredentialsStore) *ValidTokenProvider {
	return &ValidTokenProvider{
		credentialsStore: credentialsStore,
	}
}

func (v *ValidTokenProvider) GetToken() (string, error) {
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
	refreshUrl := fmt.Sprintf(constants.API_REFRESH_SESSION, constants.BaseUrl)
	err = helpers.NewUpstreamRequest(refreshUrl,
		helpers.WithHeader("Bearer", credentials.RefreshToken),
	).Send(http.MethodGet, &refreshTokenResponse)
	if err != nil {
		return fmt.Errorf("send request to refresh token: %w", err)
	}

	err = v.credentialsStore.SaveCredentials(auth.NewCredentialsFromAuthResponse(refreshTokenResponse))
	if err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	return nil
}
