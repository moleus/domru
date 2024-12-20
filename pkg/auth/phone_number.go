package auth

import (
	"fmt"
	"github.com/moleus/domru/pkg/domru/constants"
	"github.com/moleus/domru/pkg/domru/helpers"
	"github.com/moleus/domru/pkg/domru/models"
	"net/http"
	"regexp"
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

func (a *PhoneNumberAuthenticator) SubmitSmsCode(code string) (models.AuthenticationResponse, error) {
	response, err := a.sendConfirmationCode(code)
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
	confirmUrl := fmt.Sprintf("%s/auth/v2/confirmation/%s", constants.BaseUrl, a.phoneNumber)
	confirmRequest := &models.Account{
		Address:      account.Address,
		OperatorID:   account.OperatorID,
		ProfileID:    account.ProfileID,
		SubscriberID: account.SubscriberID,
	}

	err := helpers.NewUpstreamRequest(confirmUrl, helpers.WithBody(confirmRequest)).Send(http.MethodPost, nil)
	if err != nil {
		return fmt.Errorf("failed to request confirmation code: %w", err)
	}
	return nil
}

func (a *PhoneNumberAuthenticator) sendConfirmationCode(smsCode string) (models.AuthenticationResponse, error) {
	confirmUrl := fmt.Sprintf("%s/auth/v2/auth/%s/confirmation", constants.BaseUrl, a.phoneNumber)
	confirmRequest := &models.ConfirmationRequest{
		Confirm1:     smsCode,
		Confirm2:     smsCode,
		Login:        a.phoneNumber,
		OperatorID:   1,
		ProfileID:    "",
		SubscriberID: "0",
	}
	var confirmResponse models.AuthenticationResponse
	err := helpers.NewUpstreamRequest(confirmUrl, helpers.WithBody(confirmRequest)).Send(http.MethodPost, &confirmResponse)
	if err != nil {
		return models.AuthenticationResponse{}, fmt.Errorf("failed to send confirmation code: %w", err)
	}
	return confirmResponse, nil
}
