package domru

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/moleus/domru/pkg/auth"
	"github.com/moleus/domru/pkg/domru/constants"
	"github.com/moleus/domru/pkg/domru/helpers"
	myhttp "github.com/moleus/domru/pkg/domru/http"
	"github.com/moleus/domru/pkg/domru/models"
)

type APIWrapper struct {
	Logger     *slog.Logger
	baseURL    string
	authClient myhttp.HTTPClient
}

func NewDomruAPI(authClient myhttp.HTTPClient) *APIWrapper {
	return &APIWrapper{authClient: authClient, baseURL: constants.BaseUrl, Logger: slog.Default()}
}

func (w *APIWrapper) LoginWithPassword(accountID, password string) (models.AuthenticationResponse, error) {
	authenticator := auth.NewPasswordAuthenticator(accountID, password)
	authenticator.Logger = w.Logger

	return authenticator.Authenticate()
}

func (w *APIWrapper) LoginWithPhoneNumber(phoneNumber string, account models.Account) error {
	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)

	return authenticator.RequestSmsCode(account)
}

func (w *APIWrapper) SubmitSmsCode(phoneNumber, code string, account models.Account) (models.AuthenticationResponse, error) {
	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)

	return authenticator.SubmitSmsCode(code, account)
}

func (w *APIWrapper) RequestCameras() (models.CamerasResponse, error) {
	var cameras models.CamerasResponse

	camerasURL := fmt.Sprintf("%s/rest/v1/forpost/cameras", w.baseURL)
	err := helpers.NewUpstreamRequest(camerasURL, helpers.WithClient(w.authClient)).Send(http.MethodGet, &cameras)
	if err != nil {
		return models.CamerasResponse{}, fmt.Errorf("request cameras: %w", err)
	}
	return cameras, nil
}

func (w *APIWrapper) RequestPlaces() (models.PlacesResponse, error) {
	var places models.PlacesResponse

	placesURL := fmt.Sprintf("%s/rest/v1/subscriberplaces", w.baseURL)
	err := helpers.NewUpstreamRequest(placesURL, helpers.WithClient(w.authClient)).Send(http.MethodGet, &places)
	if err != nil {
		return models.PlacesResponse{}, fmt.Errorf("request places: %w", err)
	}
	return places, nil
}

func (w *APIWrapper) RequestFinances() (models.FinancesResponse, error) {
	var finances models.FinancesResponse

	financesURL := fmt.Sprintf("%s/rest/v1/subscribers/profiles/finances", w.baseURL)
	err := helpers.NewUpstreamRequest(financesURL, helpers.WithClient(w.authClient)).Send(http.MethodGet, &finances)
	if err != nil {
		return models.FinancesResponse{}, fmt.Errorf("request finances: %w", err)
	}
	return finances, nil
}

func (w *APIWrapper) RequestAccounts(phone string) ([]models.Account, error) {
	var accounts []models.Account

	loginURL := fmt.Sprintf("%s/auth/v2/login/%s", w.baseURL, phone)
	err := helpers.NewUpstreamRequest(loginURL).Send(http.MethodGet, &accounts)
	if err != nil {
		return nil, fmt.Errorf("request accounts: %w", err)
	}
	return accounts, nil
}

func (w *APIWrapper) GetSnapshot(placeID, accessControl string) ([]byte, error) {
	snapshotURL := fmt.Sprintf("%s/rest/v1/places/%s/accesscontrols/%s/videosnapshots", w.baseURL, placeID, accessControl)
	resp, err := helpers.NewUpstreamRequest(snapshotURL).SendRequest(http.MethodGet)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response content: %w", err)
	}

	contentType := http.DetectContentType(body)
	if contentType != "image/jpeg" {
		return nil, fmt.Errorf("unexpected content type: %s", contentType)
	}

	return body, nil
}

func (w *APIWrapper) GetStreamURL(cameraID string, queryParams url.Values) (string, error) {
	var videoResponse models.VideoResponse

	streamURL := fmt.Sprintf("%s/rest/v1/forpost/cameras/%s/video", w.baseURL, cameraID)
	err := helpers.NewUpstreamRequest(streamURL, helpers.WithClient(w.authClient), helpers.WithQueryParams(queryParams)).Send(http.MethodGet, &videoResponse)
	if err != nil {
		return "", fmt.Errorf("request stream streamUrl: %w", err)
	}
	if videoResponse.Data.Error != "" {
		return "", fmt.Errorf("error in response: %s", videoResponse.Data.Error)
	}

	return videoResponse.Data.URL, nil
}

func (w *APIWrapper) GetSubscriberProfile() (models.SubscriberProfilesResponse, error) {
	var profile models.SubscriberProfilesResponse

	profileURL := fmt.Sprintf("%s/rest/v1/subscribers/profiles", w.baseURL)
	err := helpers.NewUpstreamRequest(profileURL, helpers.WithClient(w.authClient)).Send(http.MethodGet, &profile)
	if err != nil {
		return models.SubscriberProfilesResponse{}, fmt.Errorf("request subscriber profile: %w", err)
	}
	return profile, nil
}
