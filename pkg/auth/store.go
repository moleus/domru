package auth

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"

	"github.com/moleus/domru/pkg/atomicfile"

	"github.com/moleus/domru/pkg/domru/models"
	"github.com/moleus/domru/pkg/domru/sanitizing_utils"
)

type Credentials struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	OperatorID   int    `json:"operatorId"`
}

func (c Credentials) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("accessToken", sanitizing_utils.KeepFirstNCharacters(c.AccessToken, 4)),
		slog.String("refreshToken", sanitizing_utils.KeepFirstNCharacters(c.RefreshToken, 4)),
		slog.Int("operatorId", c.OperatorID),
	)
}

func NewCredentialsFromAuthResponse(authResponse models.AuthenticationResponse) Credentials {
	return Credentials{
		AccessToken:  authResponse.AccessToken,
		RefreshToken: authResponse.RefreshToken,
		OperatorID:   authResponse.OperatorID,
	}
}

type CredentialsStore interface {
	SaveCredentials(credentials Credentials) error
	LoadCredentials() (Credentials, error)
}

type FileCredentialsStore struct {
	filePath string
	mu       sync.RWMutex
}

func NewFileCredentialsStore(filePath string) *FileCredentialsStore {
	return &FileCredentialsStore{filePath: filePath}
}

func (f *FileCredentialsStore) SaveCredentials(credentials Credentials) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return atomicfile.WriteJSON(f.filePath, credentials)
}

func (f *FileCredentialsStore) LoadCredentials() (Credentials, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	file, err := os.Open(f.filePath)
	if err != nil {
		return Credentials{}, err
	}
	defer file.Close()

	var credentials Credentials
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&credentials)
	if err != nil {
		return Credentials{}, err
	}

	return credentials, nil
}
