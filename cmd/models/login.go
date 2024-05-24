package models

import (
	"github.com/ad/domru/pkg/domru/models"
)

type AccountsPageData struct {
	Accounts      []models.Account
	Phone         string
	HassioIngress string
	LoginError    string
}

type LoginPageData struct {
	LoginError    string
	Phone         string
	HassioIngress string
}

type SMSPageData struct {
	Phone         string
	Index         string
	HassioIngress string
	LoginError    string
}
