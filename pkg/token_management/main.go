package token_management

import (
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/domru/helpers"
	"github.com/ad/domru/pkg/domru/models"
	"log/slog"
	"net/http"
)

type ValidTokenProvider struct {
	Logger           *slog.Logger
	credentialsStore auth.CredentialsStore
}

func NewValidTokenProvider(credentialsStore auth.CredentialsStore) *ValidTokenProvider {
	v := &ValidTokenProvider{
		credentialsStore: credentialsStore,
		Logger:           slog.Default(),
	}
	return v
}

func (v *ValidTokenProvider) GetOperatorId() (int, error) {
	credentials, err := v.credentialsStore.LoadCredentials()
	if err != nil {
		return 0, fmt.Errorf("load credentials: %w", err)
	}

	return credentials.OperatorID, nil
}

func (v *ValidTokenProvider) GetToken() (string, error) {
	credentials, err := v.credentialsStore.LoadCredentials()
	if err != nil {
		v.Logger.With("err", err.Error()).Warn("load credentials")
		return "", fmt.Errorf("load credentials: %w", err)
	}

	return credentials.AccessToken, nil
}

func (v *ValidTokenProvider) RefreshToken() error {
	v.Logger.Debug("refreshing token...")
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
