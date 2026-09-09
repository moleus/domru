package models

import "github.com/moleus/domru/pkg/domru/models"

type HomePageData struct {
	BaseURL     string
	LoginError  string
	Phone       string
	Cameras     models.CamerasResponse
	Places      models.PlacesResponse
	CameraCards []CameraCard
}

type CameraCard struct {
	ID          int
	Name        string
	Section     string
	Status      string
	SnapshotURL string
	StreamURL   string
	OpenDoorURL string
	ConfigName  string
}
