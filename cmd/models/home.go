package models

import "github.com/mrqmpan/domru/pkg/domru/models"

type HomePageData struct {
	BaseURL    string
	LoginError string
	Phone      string
	Cameras    models.CamerasResponse
	Places     models.PlacesResponse
}
