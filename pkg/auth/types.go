package auth

import (
	"github.com/moleus/domru/pkg/domru/models"
	"github.com/moleus/domru/pkg/domru/sanitizing_utils"
	"log/slog"
)

type Authentication interface {
	Prepare() error
	Authenticate() (models.AuthenticationResponse, error)
}

type PasswordAuthRequest struct {
	Hash1     string `json:"hash1"`
	Hash2     string `json:"hash2"`
	Login     string `json:"login"`
	Timestamp string `json:"timestamp"`
}

func (a PasswordAuthRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("hash1", sanitizing_utils.KeepFirstNCharacters(a.Hash1, 4)),
		slog.String("hash2", sanitizing_utils.KeepFirstNCharacters(a.Hash2, 4)),
		slog.String("login", sanitizing_utils.KeepFirstNCharacters(a.Login, 4)),
		slog.String("timestamp", a.Timestamp),
	)
}
