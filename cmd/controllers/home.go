package controllers

import (
	errors2 "errors"
	"net/http"
	"strings"

	"github.com/moleus/domru/cmd/models"
	"github.com/moleus/domru/pkg/authorizedhttp"
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

	errorsMessage := strings.Join(errors, "\n")

	data.BaseURL = h.determineBaseURL(r)
	data.LoginError = errorsMessage

	return data, nil
}
