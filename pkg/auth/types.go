package auth

import "github.com/ad/domru/pkg/domru/models"

type Authentication interface {
	Prepare() error
	Authenticate() (models.AuthenticationResponse, error)
}
