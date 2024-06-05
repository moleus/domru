package auth

import (
	"testing"
	"time"
)

func TestGeneratePasswordAuthRequest(t *testing.T) {
	login := "780000000000"
	password := "secret"
	parsedTime, err := time.Parse(TimestampLayout, "2022-08-25T19:30:46.409Z")
	if err != nil {
		t.Errorf("Failed to parse time: %v", err)
	}
	timestamp := parsedTime.Format(TimestampLayout)

	request := generatePasswordAuthRequest(parsedTime, login, password)

	if request.Login != login {
		t.Errorf("Expected login %s, but got %s", login, request.Login)
	}

	if request.Timestamp != timestamp {
		t.Errorf("Timestamp is not in the correct format: %v", err)
	}

	if request.Hash1 != "5en6G6MezRroT3XKqkdPOmY/BfQ=" {
		t.Errorf("Hash1 is not correct")
	}

	if request.Hash2 != "9c81153e0c398180e7284d6b77c47d39" {
		t.Errorf("Hash2 is not correct")
	}
}
