package auth

type Authentication interface {
	Prepare() error
	Authenticate() (AuthenticationResponse, error)
}

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
