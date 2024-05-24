package controllers

import (
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/pkg/home_assistant"
	"log"
	"net/http"
	"strings"
)

func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	data := h.prepareHomePageData(r)

	err := h.renderTemplate(w, "home", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) prepareHomePageData(r *http.Request) models.HomePageData {
	var errors []string
	data := models.HomePageData{}

	hostIP, haNetworkErr := home_assistant.GetHomeAssistantNetworkAddress()
	if haNetworkErr != nil {
		log.Printf("Failed to get Home Assistant network address: %v", haNetworkErr)
	} else {
		data.HostIP = hostIP
	}

	cameras, camerasErr := h.domruApi.RequestCameras()
	if camerasErr != nil {
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

	data.Host = r.Host
	if data.Scheme = r.URL.Scheme; data.Scheme == "" {
		data.Scheme = "http"
	}

	errorsMessage := strings.Join(errors, "\n")

	data.HassioIngress = r.Header.Get("X-Ingress-Path")
	// TODO: set phone number
	data.Phone = "TODO: set phone number"

	data.LoginError = errorsMessage

	return data
}
