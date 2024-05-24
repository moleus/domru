package controllers

import (
	"errors"
	"fmt"
	"github.com/ad/domru/cmd/models"
	"github.com/ad/domru/handlers"
	"net/http"
	"strconv"
)

func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if h.Config.Token == "" || h.Config.RefreshToken == "" {
		http.Redirect(w, r, r.Header.Get("X-Ingress-Path")+"/login", http.StatusSeeOther)
		return
	}

	data := h.prepareHomePageData(r)

	err := h.renderTemplate(w, "home", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) prepareHomePageData(r *http.Request) models.HomePageData {
	data := models.HomePageData{}

	hostIP, haNetworkErr := handlers.GetHomeAssistantNetworkAddress()
	if haNetworkErr == nil {
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

	if data.Host = r.Host; data.Host == "" {
		data.Host = fmt.Sprintf("%s:%s", data.HostIP, strconv.Itoa(h.Config.Port))
	}

	if data.Scheme = r.URL.Scheme; data.Scheme == "" {
		data.Scheme = "http"
	}

	data.HassioIngress = r.Header.Get("X-Ingress-Path")
	data.Port = strconv.Itoa(h.Config.Port)
	data.Phone = strconv.Itoa(h.Config.Login)
	data.Token = h.Config.Token
	data.RefreshToken = h.Config.RefreshToken
	data.LoginError = errors.Join(haNetworkErr, camerasErr, financesErr, placesErr).Error()

	return data
}
