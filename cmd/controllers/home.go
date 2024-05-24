package controllers

import (
	"errors"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/home_assistant"
	"log"
	"net/http"
)

func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	data := h.prepareHomePageData(r)

	err := h.renderTemplate(w, "home", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) prepareHomePageData(r *http.Request) models.HomePageData {
	data := models.HomePageData{}

	hostIP, haNetworkErr := home_assistant.GetHomeAssistantNetworkAddress()
	if haNetworkErr != nil {
		log.Printf("Failed to get Home Assistant network address: %v", haNetworkErr)
	} else {
		data.HostIP = hostIP
	}

	cameras, camerasErr := h.domruApi.RequestCameras()
	if camerasErr == nil {
		data.Cameras = cameras
	}

	finances, financesErr := h.domruApi.RequestFinances()
	if financesErr == nil {
		data.Finances = finances
	}

	places, placesErr := h.domruApi.RequestPlaces()
	if placesErr == nil {
		data.Places = places
	}

	data.Host = r.Host

	if data.Scheme = r.URL.Scheme; data.Scheme == "" {
		data.Scheme = "http"
	}

	data.HassioIngress = r.Header.Get("X-Ingress-Path")
	// TODO: set phone number
	data.Phone = "TODO: set phone number"
	data.LoginError = errors.Join(camerasErr, financesErr, placesErr).Error()

	return data
}
