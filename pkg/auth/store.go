package auth

import (
	"encoding/json"
	"os"
)

type CredentialsStore interface {
	SaveCredentials(credentials AuthenticationResponse) error
	LoadCredentials() (AuthenticationResponse, error)
}

type FileCredentialsStore struct {
	filePath string
}

func NewFileCredentialsStore(filePath string) *FileCredentialsStore {
	return &FileCredentialsStore{filePath: filePath}
}

func (f *FileCredentialsStore) SaveCredentials(credentials AuthenticationResponse) error {
	file, err := os.OpenFile(f.filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(credentials)
}

func (f *FileCredentialsStore) LoadCredentials() (AuthenticationResponse, error) {
	file, err := os.Open(f.filePath)
	if err != nil {
		return AuthenticationResponse{}, err
	}
	defer file.Close()

	var credentials AuthenticationResponse
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&credentials)
	if err != nil {
		return AuthenticationResponse{}, err
	}

	return credentials, nil
}
