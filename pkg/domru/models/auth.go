package models

import (
	"log/slog"
)

type AuthenticationResponse struct {
	AccessToken      string  `json:"accessToken"`
	ExpiresIn        *int    `json:"expiresIn"`
	OperatorID       int     `json:"operatorId"`
	OperatorName     string  `json:"operatorName"`
	RefreshExpiresIn *int    `json:"refreshExpiresIn"`
	RefreshToken     string  `json:"refreshToken"`
	TokenType        *string `json:"tokenType"`
}

type ConfirmationRequest struct {
	Confirm1     string `json:"confirm1"`
	Confirm2     string `json:"confirm2"`
	Login        string `json:"login"`
	OperatorID   int    `json:"operatorId"`
	ProfileID    string `json:"profileId"`
	SubscriberID string `json:"subscriberId"`
}

type Account struct {
	AccountID    *string `json:"accountId"`
	Address      string  `json:"address"`
	OperatorID   int     `json:"operatorId"`
	PlaceID      int     `json:"placeId"`
	ProfileID    *string `json:"profileId"` // Use pointer to string for null value
	SubscriberID int     `json:"subscriberId"`
}

func (a AuthenticationResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("accessToken", "[REDACTED]"),
		slog.Int("expiresIn", *a.ExpiresIn),
		slog.Int("operatorId", a.OperatorID),
		slog.String("operatorName", a.OperatorName),
		slog.Int("refreshExpiresIn", *a.RefreshExpiresIn),
		slog.String("refreshToken", "[REDACTED]"),
		slog.String("tokenType", *a.TokenType),
	)
}
