package auth

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/moleus/domru/pkg/domru/constants"
	"github.com/moleus/domru/pkg/domru/helpers"
	"github.com/moleus/domru/pkg/domru/models"
)

const (
	phoneNumberRegex = `^\+7\d{10}$`
)

type SmsCodeGetter func() (string, error)

type PhoneNumberAuthenticator struct {
	phoneNumber string
}

func NewPhoneNumberAuthenticator(phoneNumber string) *PhoneNumberAuthenticator {
	return &PhoneNumberAuthenticator{
		phoneNumber: phoneNumber,
	}
}

func (a *PhoneNumberAuthenticator) RequestSmsCode(account models.Account) error {
	if !a.isPhoneNumberValid() {
		return fmt.Errorf("phone number is invalid format. It should be +7XXXXXXXXXX")
	}

	if err := a.requestConfirmationCode(account); err != nil {
		return fmt.Errorf("failed to request confirmation code: %w", err)
	}
	return nil
}

func (a *PhoneNumberAuthenticator) SubmitSmsCode(code string, account models.Account) (models.AuthenticationResponse, error) {
	response, err := a.sendConfirmationCode(code, account)
	if err != nil {
		return models.AuthenticationResponse{}, fmt.Errorf("failed to authenticate: %w", err)
	}

	return response, nil
}

func (a *PhoneNumberAuthenticator) isPhoneNumberValid() bool {
	r, err := regexp.Compile(phoneNumberRegex)
	if err != nil {
		panic(err)
	}

	return r.MatchString(a.phoneNumber)
}

func (a *PhoneNumberAuthenticator) requestConfirmationCode(account models.Account) error {
	confirmURL := fmt.Sprintf("%s/auth/v2/confirmation/%s", constants.BaseUrl, a.phoneNumber)
	if account.AccountID == nil {
		return fmt.Errorf("account id is nil. Account: %v", account)
	}

	if account.ProfileID == nil {
		return fmt.Errorf("profile id is nil. Account: %v", account)
	}

	err := helpers.NewUpstreamRequest(confirmURL, helpers.WithBody(account)).Send(http.MethodPost, nil)
	if err != nil {
		return fmt.Errorf("failed to request confirmation code: %w", err)
	}
	return nil
}

func (a *PhoneNumberAuthenticator) sendConfirmationCode(smsCode string, account models.Account) (models.AuthenticationResponse, error) {
	confirmURL := fmt.Sprintf("%s/auth/v3/auth/%s/confirmation", constants.BaseUrl, a.phoneNumber)
	if account.ProfileID == nil {
		return models.AuthenticationResponse{}, fmt.Errorf("profile id is nil. Account: %v", account)
	}
	if account.AccountID == nil {
		return models.AuthenticationResponse{}, fmt.Errorf("account id is nil. Account: %v", account)
	}
	var confirmRequest = &models.ConfirmationRequest{
		OperatorID:   account.OperatorID,
		Login:        a.phoneNumber,
		AccountID:    *account.AccountID,
		Confirm1:     smsCode,
		Confirm2:     smsCode,
		SubscriberID: strconv.Itoa(account.SubscriberID),
	}
	var confirmResponse models.AuthenticationResponse
	err := helpers.NewUpstreamRequest(confirmURL, helpers.WithBody(confirmRequest)).Send(http.MethodPost, &confirmResponse)
	if err != nil {
		return models.AuthenticationResponse{}, fmt.Errorf("failed to send confirmation code: %w", err)
	}
	return confirmResponse, nil
}
