package models

import (
	"github.com/ad/domru/pkg/domru/models"
)

type AccountsPageData struct {
	Accounts   []models.Account
	Phone      string
	BaseUrl    string
	LoginError string
}

type LoginPageData struct {
	LoginError string
	Phone      string
	BaseUrl    string
}

type SMSPageData struct {
	Phone      string
	BaseUrl    string
	LoginError string
}
