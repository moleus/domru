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

type VideoResponse struct {
	Data struct {
		URL       string `json:"URL"`
		Error     string `json:"Error"`
		ErrorCode string `json:"ErrorCode"`
		Status    string `json:"Status"`
	} `json:"data"`
}
