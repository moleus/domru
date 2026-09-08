package controllers

import (
	errors2 "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/authorizedhttp"
	"github.com/moleus/domru/pkg/domru/helpers"
	domrumodels "github.com/moleus/domru/pkg/domru/models"
)

func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	data, err := h.prepareHomePageData(r)
	if errors2.As(err, &authorizedhttp.TokenRefreshError{}) {
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		return
	}

	err = h.renderTemplate(w, "home", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) prepareHomePageData(r *http.Request) (models.HomePageData, error) {
	var errors []string
	data := models.HomePageData{}

	cameras, camerasErr := h.domruAPI.RequestCameras()
	if camerasErr != nil {
		if errors2.As(camerasErr, &authorizedhttp.TokenRefreshError{}) {
			return data, camerasErr
		}
		errors = append(errors, camerasErr.Error())
	} else {
		data.Cameras = cameras
	}

	places, placesErr := h.domruAPI.RequestPlaces()
	if placesErr != nil {
		errors = append(errors, placesErr.Error())
	} else {
		data.Places = places
	}

	subscriberProfiles, subscriberProfilesErr := h.domruAPI.GetSubscriberProfile()
	if subscriberProfilesErr != nil {
		errors = append(errors, subscriberProfilesErr.Error())
	} else {
		if len(subscriberProfiles.SubscriberPhones) > 0 {
			data.Phone = subscriberProfiles.SubscriberPhones[0].Number
		}
	}

	data.BaseURL = h.determineBaseURL(r)
	sections := make(map[int]domrumodels.ScreenSectionsResponse)
	controlsByPlace := make(map[int][]domrumodels.AccessControl)
	for i, item := range data.Places.Data {
		placeID := item.Place.ID
		controls, exists := controlsByPlace[placeID]
		if !exists {
			result, err := h.domruAPI.RequestAccessControls(placeID)
			controls = result.Data
			if err != nil {
				// The legacy embedded list can incorrectly advertise door access
				// for paid neighboring cameras. Keep video metadata only on failure.
				controls = append([]domrumodels.AccessControl(nil), item.Place.AccessControls...)
				for j := range controls {
					controls[j].AllowOpen = false
				}
				errors = append(errors, fmt.Sprintf("Не удалось проверить доступ к домофонам адреса %d: %v", placeID, err))
			}
			controlsByPlace[placeID] = controls
		}
		data.Places.Data[i].Place.AccessControls = controls
		if _, exists := sections[placeID]; exists {
			continue
		}
		result, err := h.domruAPI.RequestScreenSections(placeID)
		sections[placeID] = result
		if err != nil {
			// Older operators may not expose this optional endpoint.
			var upstreamErr *helpers.UpstreamError
			if errors2.As(err, &upstreamErr) && upstreamErr.StatusCode == http.StatusNotFound {
				continue
			}
			errors = append(errors, fmt.Sprintf("Не удалось загрузить дополнительные камеры адреса %d: %v", placeID, err))
		}
	}
	data.CameraCards = buildCameraCards(data.BaseURL, data.Places, data.Cameras, sections, h.EndCallDoor)
	data.LoginError = strings.Join(errors, "\n")

	return data, nil
}
