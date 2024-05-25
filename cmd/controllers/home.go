package controllers

import (
	errors2 "errors"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/authorizedhttp"
	"net/http"
	"strings"
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

	cameras, camerasErr := h.domruApi.RequestCameras()
	if camerasErr != nil {
		if errors2.As(camerasErr, &authorizedhttp.TokenRefreshError{}) {
			return data, camerasErr
		}
		errors = append(errors, camerasErr.Error())
	} else {
		data.Cameras = cameras
	}

	places, placesErr := h.domruApi.RequestPlaces()
	if placesErr != nil {
		errors = append(errors, placesErr.Error())
	} else {
		data.Places = places
	}

	errorsMessage := strings.Join(errors, "\n")

	data.BaseUrl = h.determineBaseUrl(r)
	// TODO: set phone number
	data.Phone = "TODO: set phone number"
	data.LoginError = errorsMessage

	return data, nil
}
