package auth

import (
	"encoding/json"
	"github.com/ad/domru/pkg/domru/models"
	"os"
)

type CredentialsStore interface {
	SaveCredentials(credentials models.AuthenticationResponse) error
	LoadCredentials() (models.AuthenticationResponse, error)
}

type FileCredentialsStore struct {
	filePath string
}

func NewFileCredentialsStore(filePath string) *FileCredentialsStore {
	return &FileCredentialsStore{filePath: filePath}
}

func (f *FileCredentialsStore) SaveCredentials(credentials models.AuthenticationResponse) error {
	file, err := os.OpenFile(f.filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(credentials)
}

func (f *FileCredentialsStore) LoadCredentials() (models.AuthenticationResponse, error) {
	file, err := os.Open(f.filePath)
	if err != nil {
		return models.AuthenticationResponse{}, err
	}
	defer file.Close()

	var credentials models.AuthenticationResponse
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&credentials)
	if err != nil {
		return models.AuthenticationResponse{}, err
	}

	return credentials, nil
}
