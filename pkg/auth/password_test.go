package auth

import (
	"testing"
	"time"
)

func TestGeneratePasswordAuthRequest(t *testing.T) {
	login := "780000000000"
	password := "secret"
	earthTimestamp := "20220825193046"

	request := generatePasswordAuthRequest(login, password)

	if request.Login != login {
		t.Errorf("Expected login %s, but got %s", login, request.Login)
	}

	_, err := time.Parse("20060102150405", request.Timestamp)
	if err != nil {
		t.Errorf("Timestamp is not in the correct format: %v", err)
	}

	if request.Hash1 != "5en6G6MezRroT3XKqkdPOmY/BfQ=" {
		t.Errorf("Hash1 is not correct")
	}

	if hash2(login, password, earthTimestamp) != "9c81153e0c398180e7284d6b77c47d39" {
		t.Errorf("Hash2 is not correct")
	}
}
