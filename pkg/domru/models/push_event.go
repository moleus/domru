package models

type PushEvent struct {
	Event struct {
		Payload Payload
		Type    string `json:"type"`
	}
}

type Payload struct {
	Actions       []Action `json:"actions"` // Assuming Action is a predefined struct in Go
	EventTypeName string   `json:"event_type_name"`
	ID            int      `json:"id"`
	Message       string   `json:"message"`
	PlaceID       int      `json:"place_id"`
	Source        Source   `json:"source"` // Ensure there's a corresponding Go struct for Source
	Timestamp     int64    `json:"timestamp"`
}

type Action string

type Source struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
}
