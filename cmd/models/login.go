package models

import (
	"github.com/moleus/domru/pkg/domru/models"
)

type AccountsPageData struct {
	Accounts   []models.Account
	Phone      string
	BaseURL    string
	LoginError string
}

type LoginPageData struct {
	LoginError string
	Phone      string
	BaseURL    string
}

type SMSPageData struct {
	Phone      string
	BaseURL    string
	LoginError string
}
