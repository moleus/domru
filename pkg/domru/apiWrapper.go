package domru

import (
	"fmt"
	"github.com/ad/domru/pkg/auth"
	"github.com/ad/domru/pkg/domru/constants"
	"github.com/ad/domru/pkg/domru/helpers"
	myhttp "github.com/ad/domru/pkg/domru/http"
	"github.com/ad/domru/pkg/domru/models"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

type APIWrapper struct {
	Logger     *slog.Logger
	baseUrl    string
	authClient myhttp.HTTPClient
}

func NewDomruAPI(authClient myhttp.HTTPClient) *APIWrapper {
	return &APIWrapper{authClient: authClient, baseUrl: constants.BaseUrl, Logger: slog.Default()}
}

func (w *APIWrapper) LoginWithPassword(accountId, password string) (models.AuthenticationResponse, error) {

	authenticator := auth.NewPasswordAuthenticator(accountId, password)
	authenticator.Logger = w.Logger

	return authenticator.Authenticate()
}

func (w *APIWrapper) LoginWithPhoneNumber(phoneNumber string, account models.Account) error {
	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)

	return authenticator.RequestSmsCode(account)
}

func (w *APIWrapper) SubmitSmsCode(phoneNumber, code string) (models.AuthenticationResponse, error) {
	authenticator := auth.NewPhoneNumberAuthenticator(phoneNumber)

	return authenticator.SubmitSmsCode(code)
}

func (w *APIWrapper) RequestCameras() (models.CamerasResponse, error) {
	var cameras models.CamerasResponse

	camerasUrl := fmt.Sprintf("%s/rest/v1/forpost/cameras", w.baseUrl)
	err := helpers.NewUpstreamRequest(camerasUrl, helpers.WithClient(w.authClient)).Send(http.MethodGet, &cameras)
	if err != nil {
		return models.CamerasResponse{}, fmt.Errorf("request cameras: %w", err)
	}
	return cameras, nil
}

func (w *APIWrapper) RequestPlaces() (models.PlacesResponse, error) {
	var places models.PlacesResponse

	placesUrl := fmt.Sprintf("%s/rest/v1/subscriberplaces", w.baseUrl)
	err := helpers.NewUpstreamRequest(placesUrl, helpers.WithClient(w.authClient)).Send(http.MethodGet, &places)
	if err != nil {
		return models.PlacesResponse{}, fmt.Errorf("request places: %w", err)
	}
	return places, nil
}

func (w *APIWrapper) RequestFinances() (models.FinancesResponse, error) {
	var finances models.FinancesResponse

	financesUrl := fmt.Sprintf("%s/rest/v1/subscribers/profiles/finances", w.baseUrl)
	err := helpers.NewUpstreamRequest(financesUrl, helpers.WithClient(w.authClient)).Send(http.MethodGet, &finances)
	if err != nil {
		return models.FinancesResponse{}, fmt.Errorf("request finances: %w", err)
	}
	return finances, nil
}

func (w *APIWrapper) RequestAccounts(phone string) ([]models.Account, error) {
	var accounts []models.Account

	loginUrl := fmt.Sprintf("%s/auth/v2/login/%s", w.baseUrl, phone)
	err := helpers.NewUpstreamRequest(loginUrl).Send(http.MethodGet, &accounts)
	if err != nil {
		return nil, fmt.Errorf("request accounts: %w", err)
	}
	return accounts, nil
}

func (w *APIWrapper) GetSnapshot(placeId, accessControl string) ([]byte, error) {
	snapshotUrl := fmt.Sprintf("%s/rest/v1/places/%s/accesscontrols/%s/videosnapshots", w.baseUrl, placeId, accessControl)
	resp, err := helpers.NewUpstreamRequest(snapshotUrl).SendRequest(http.MethodGet)
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

func (w *APIWrapper) GetStreamUrl(cameraId string, queryParams url.Values) (string, error) {
	var videoResponse models.VideoResponse

	streamUrl := fmt.Sprintf("%s/rest/v1/forpost/cameras/%s/video", w.baseUrl, cameraId)
	err := helpers.NewUpstreamRequest(streamUrl, helpers.WithClient(w.authClient), helpers.WithQueryParams(queryParams)).Send(http.MethodGet, &videoResponse)
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

	profileUrl := fmt.Sprintf("%s/rest/v1/subscribers/profiles", w.baseUrl)
	err := helpers.NewUpstreamRequest(profileUrl, helpers.WithClient(w.authClient)).Send(http.MethodGet, &profile)
	if err != nil {
		return models.SubscriberProfilesResponse{}, fmt.Errorf("request subscriber profile: %w", err)
	}
	return profile, nil
}
