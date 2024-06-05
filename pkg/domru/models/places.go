package models

type KladrAddress struct {
	Index          interface{} `json:"index"`
	Region         interface{} `json:"region"`
	District       interface{} `json:"district"`
	City           string      `json:"city"`
	Locality       interface{} `json:"locality"`
	Street         string      `json:"street"`
	House          string      `json:"house"`
	Building       interface{} `json:"building"`
	Apartment      string      `json:"apartment"`
	VisibleAddress string      `json:"visibleAddress"`
	GroupName      string      `json:"groupName"`
}

type Address struct {
	KladrAddress       KladrAddress `json:"kladrAddress"`
	KladrAddressString string       `json:"kladrAddressString"`
	VisibleAddress     string       `json:"visibleAddress"`
	GroupName          string       `json:"groupName"`
}

type Location struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type AccessControl struct {
	ID                     int           `json:"id"`
	OperatorID             int           `json:"operatorId"`
	Name                   string        `json:"name"`
	ForpostGroupId         string        `json:"forpostGroupId"`
	ForpostAccountId       interface{}   `json:"forpostAccountId"`
	Type                   string        `json:"type"`
	AllowOpen              bool          `json:"allowOpen"`
	OpenMethod             string        `json:"openMethod"`
	AllowVideo             bool          `json:"allowVideo"`
	AllowCallMobile        bool          `json:"allowCallMobile"`
	AllowSlideshow         bool          `json:"allowSlideshow"`
	PreviewAvailable       bool          `json:"previewAvailable"`
	VideoDownloadAvailable bool          `json:"videoDownloadAvailable"`
	TimeZone               int           `json:"timeZone"`
	Quota                  interface{}   `json:"quota"`
	ExternalCameraId       interface{}   `json:"externalCameraId"`
	ExternalDeviceId       interface{}   `json:"externalDeviceId"`
	Entrances              []interface{} `json:"entrances"`
}

type Place struct {
	ID                     int             `json:"id"`
	Address                Address         `json:"address"`
	Location               *Location       `json:"location"`
	AutoArmingState        bool            `json:"autoArmingState"`
	AutoArmingRadius       int             `json:"autoArmingRadius"`
	PreviewAvailable       bool            `json:"previewAvailable"`
	VideoDownloadAvailable bool            `json:"videoDownloadAvailable"`
	Controllers            []interface{}   `json:"controllers"`
	AccessControls         []AccessControl `json:"accessControls"`
	Cameras                []interface{}   `json:"cameras"`
}

type Payment struct {
	UseLink bool `json:"useLink"`
}

type Data struct {
	ID              int         `json:"id"`
	SubscriberType  string      `json:"subscriberType"`
	SubscriberState string      `json:"subscriberState"`
	Place           Place       `json:"place"`
	Subscriber      Subscriber  `json:"subscriber"`
	GuardCallOut    interface{} `json:"guardCallOut"`
	Payment         Payment     `json:"payment"`
	Blocked         bool        `json:"blocked"`
}

type PlacesResponse struct {
	Data []Data `json:"data"`
}
