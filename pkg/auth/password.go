package auth

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/ad/domru/pkg/domru/helpers"
	"github.com/ad/domru/pkg/domru/models"
	"net/http"
	"time"
)

const (
	AUTH_PASSWORD_URL = "https://myhome.proptech.ru/auth/v2/auth/%s/password"
)

func getAuthPasswordUrl(login string) string {
	return fmt.Sprintf(AUTH_PASSWORD_URL, login)
}

type PasswordAuthenticator struct {
	login    string
	password string
}

func NewPasswordAuthenticator(login, password string) *PasswordAuthenticator {
	return &PasswordAuthenticator{
		login:    login,
		password: password,
	}
}

func (a *PasswordAuthenticator) Authenticate() (models.AuthenticationResponse, error) {
	body := generatePasswordAuthRequest(a.login, a.password)

	var authResp models.AuthenticationResponse
	url := getAuthPasswordUrl(a.login)
	err := helpers.NewUpstreamRequest(url, helpers.WithBody(body)).Send(http.MethodPost, &authResp)
	if err != nil {
		return models.AuthenticationResponse{}, fmt.Errorf("auth password request: %w", err)
	}

	return authResp, nil
}

type PasswordAuthRequest struct {
	Login     string `json:"login"`
	Timestamp string `json:"timestamp"`
	Hash1     string `json:"hash1"`
	Hash2     string `json:"hash2"`
}

func generatePasswordAuthRequest(login, password string) PasswordAuthRequest {
	timestamp := time.Now().Format("20060102150405") // yyyyMMddHHmmss

	hash1 := hash1(password)
	hash2 := hash2(login, password, timestamp)

	return PasswordAuthRequest{
		Login:     login,
		Timestamp: timestamp,
		Hash1:     hash1,
		Hash2:     hash2,
	}
}

func hash1(textPassword string) string {
	return encodeSHA1(textPassword)
}

func encodeSHA1(input string) string {
	hasher := sha1.New()
	hasher.Write([]byte(input))
	sha := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return sha
}

func hash2(account, password, earthTimestamp string) string {
	return encodeMD5("DigitalHomeNTK", "password", account, password, earthTimestamp, "789sdgHJs678wertv34712376")
}

func encodeMD5(stringFragments ...string) string {
	concatenated := ""
	for _, str := range stringFragments {
		concatenated += str
	}

	hasher := md5.New()
	hasher.Write([]byte(concatenated))
	encodedHash := hex.EncodeToString(hasher.Sum(nil))
	return encodedHash
}
