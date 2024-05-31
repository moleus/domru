package auth

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/ad/domru/pkg/antiblock_client"
	"github.com/ad/domru/pkg/domru/helpers"
	"github.com/ad/domru/pkg/domru/models"
	"log/slog"
	"net/http"
	"time"
)

const (
	AuthPasswordUrl      = "https://myhome.proptech.ru/auth/v2/auth/%s/password"
	TimestampLayout      = "2006-01-02T15:04:05.999Z"
	EarthTimestampLayout = "20060102150405"
)

func getAuthPasswordUrl(login string) string {
	return fmt.Sprintf(AuthPasswordUrl, login)
}

type PasswordAuthenticator struct {
	Logger   *slog.Logger
	login    string
	password string
}

func NewPasswordAuthenticator(login, password string) *PasswordAuthenticator {
	return &PasswordAuthenticator{
		Logger:   slog.Default(),
		login:    login,
		password: password,
	}
}

func (a *PasswordAuthenticator) Authenticate() (models.AuthenticationResponse, error) {
	now := time.Now()
	body := generatePasswordAuthRequest(now, a.login, a.password)

	var authResp models.AuthenticationResponse
	url := getAuthPasswordUrl(a.login)
	antiblockClient := antiblock_client.NewAntiblockClient()
	err := helpers.NewUpstreamRequest(url,
		helpers.WithBody(body),
		helpers.WithLogger(a.Logger),
		helpers.WithClient(antiblockClient),
	).Send(http.MethodPost, &authResp)
	if err != nil {
		a.Logger.With("url", url).With("body", body).With("error", err).Error("auth password request")
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

func generatePasswordAuthRequest(now time.Time, login, password string) PasswordAuthRequest {
	timestamp := now.Format(TimestampLayout)
	earthTimestamp := now.Format(EarthTimestampLayout)
	hash1 := hash1(password)
	hash2 := hash2(login, password, earthTimestamp)

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
