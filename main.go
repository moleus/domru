package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/ad/domru/config"
	"github.com/ad/domru/handlers"
	"github.com/hashicorp/go-retryablehttp"
)

//go:embed templates/*
var templateFs embed.FS

func main() {
	// Init Config
	addonConfig := config.InitConfig()

    httpClient := retryablehttp.NewClient()
    httpClient.RetryMax = 5

    standartClient := httpClient.StandardClient()

	// Init Handlers
	h := handlers.NewHandlers(addonConfig, templateFs, standartClient)

	switch {
	case addonConfig.Token != "" || addonConfig.RefreshToken != "":
		if addonConfig.RefreshToken != "" {
			access, refresh, err := h.Refresh(&addonConfig.RefreshToken)
			if err != nil {
				log.Println("refresh token, error:", err.Error())
                os.Exit(1);
			} else {
				addonConfig.Token = access
				addonConfig.RefreshToken = refresh

				if err = addonConfig.WriteConfig(); err != nil {
					log.Println("error on write config file ", err)
				}
			}
		}
	default:
		log.Println("auth/refresh token or login and password must be provided")
	}

	http.HandleFunc("/", h.HomeHandler)
	http.HandleFunc("/login", h.LoginHandler)
	http.HandleFunc("/login/address", h.LoginAddressHandler)
	http.HandleFunc("/sms", h.LoginSMSHandler)
	// http.HandleFunc("/network", h.HANetworkHandler)

	http.HandleFunc("/cameras", h.CamerasHandler)
	http.HandleFunc("/door", h.DoorHandler)
	http.HandleFunc("/events/last", h.LastEventHandler)
	http.HandleFunc("/events", h.EventsHandler)
	http.HandleFunc("/finances", h.FinancesHandler)
	http.HandleFunc("/operators", h.OperatorsHandler)
	http.HandleFunc("/places", h.PlacesHandler)
	http.HandleFunc("/snapshot", h.SnapshotHandler)
	http.HandleFunc("/stream", h.StreamHandler)

	log.Println("start listening on", addonConfig.Port, "with token", addonConfig.Token)

	if err := http.ListenAndServe(":"+strconv.Itoa(addonConfig.Port), nil); err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}
