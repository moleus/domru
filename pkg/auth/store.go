package auth

import (
	"encoding/json"
	"github.com/ad/domru/pkg/domru/models"
	"github.com/ad/domru/pkg/domru/sanitizing_utils"
	"log/slog"
	"os"
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
}

func NewFileCredentialsStore(filePath string) *FileCredentialsStore {
	return &FileCredentialsStore{filePath: filePath}
}

func (f *FileCredentialsStore) SaveCredentials(credentials Credentials) error {
	file, err := os.OpenFile(f.filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(credentials)
}

func (f *FileCredentialsStore) LoadCredentials() (Credentials, error) {
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
