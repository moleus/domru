package models

import "github.com/ad/domru/pkg/domru/models"

type HomePageData struct {
	HassioIngress string
	HostIP        string
	Port          string
	Host          string
	Scheme        string
	LoginError    string
	Phone         string
	Token         string
	RefreshToken  string
	Cameras       models.CamerasResponse
	Places        models.PlacesResponse
	Finances      models.FinancesResponse
}
