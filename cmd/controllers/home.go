package controllers

import (
	"fmt"
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

	errorsMessage := strings.Join(errors, "\n")

	data.BaseUrl = h.determineBaseUrl(r)
	// TODO: set phone number
	data.Phone = "TODO: set phone number"
	data.LoginError = errorsMessage

	return data
}

func (h *Handler) determineBaseUrl(r *http.Request) string {
	var scheme string
	var host string

	if scheme = r.URL.Scheme; scheme == "" {
		scheme = "http"
	}
	haHost, haNetworkErr := home_assistant.GetHomeAssistantNetworkAddress()
	if haNetworkErr != nil {
		host = r.Host
	}
	ingressPath := r.Header.Get("X-Ingress-Path")
	if ingressPath == "" && haHost != "" {
		log.Printf("[WARNING] X-Ingress-Path header is empty, when using Home Assistant host %s", haHost)
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, ingressPath)
}
