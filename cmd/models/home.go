package models

import "github.com/ad/domru/pkg/domru/models"

type HomePageData struct {
	BaseUrl    string
	LoginError string
	Phone      string
	Cameras    models.CamerasResponse
	Places     models.PlacesResponse
}
