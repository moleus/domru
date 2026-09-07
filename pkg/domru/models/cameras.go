package models

type Camera struct {
	ID                 int           `json:"ID"`
	Name               string        `json:"Name"`
	IsActive           int           `json:"IsActive"`
	IsSound            int           `json:"IsSound"`
	RecordType         int           `json:"RecordType"`
	Quota              int           `json:"Quota"`
	MaxBandwidth       interface{}   `json:"MaxBandwidth"`
	HomeMode           int           `json:"HomeMode"`
	Devices            interface{}   `json:"Devices"`
	ParentGroups       []ParentGroup `json:"ParentGroups"`
	State              int           `json:"State"`
	TimeZone           int           `json:"TimeZone"`
	MotionDetectorMode string        `json:"MotionDetectorMode"`
	ParentID           string        `json:"ParentID"`
}

type ParentGroup struct {
	ID       int    `json:"ID"`
	Name     string `json:"Name"`
	ParentID int    `json:"ParentID"`
}

type CamerasResponse struct {
	Data []Camera `json:"data"`
}

type ScreenSectionsResponse struct {
	Sections []ScreenSection `json:"sections"`
}

type ScreenSection struct {
	Order    int                   `json:"order"`
	Title    string                `json:"title"`
	Type     string                `json:"type"`
	Entities []AccessControlCamera `json:"entities"`
}

// AccessControlCamera describes a viewing entitlement, not permission to open a door.
type AccessControlCamera struct {
	AccessControlID  int    `json:"accessControlId"`
	Name             string `json:"name"`
	AllowVideo       bool   `json:"allowVideo"`
	AllowSlideshow   bool   `json:"allowSlideshow"`
	PreviewAvailable bool   `json:"previewAvailable"`
	ExternalCameraID string `json:"externalCameraId"`
	ServiceActivated bool   `json:"serviceActivated"`
}

type VideoResponse struct {
	Data struct {
		URL       string `json:"URL"`
		Error     string `json:"Error"`
		ErrorCode string `json:"ErrorCode"`
		Status    string `json:"Status"`
	} `json:"data"`
}
