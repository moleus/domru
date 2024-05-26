package models

type FinancesResponse struct {
	Balance       float64 `json:"balance"`
	BlockType     string  `json:"blockType"`
	AmountSum     float64 `json:"amountSum"`
	TargetDate    string  `json:"targetDate"`
	PaymentLink   string  `json:"paymentLink"`
	DaysToBlock   *int    `json:"daysToBlock"`
	DaysToWarning *int    `json:"daysToWarning"`
	Blocked       bool    `json:"blocked"`
}

/*
{
    "data": {
        "allowAddPhone": false,
        "callSelectedPlaceOnly": false,
        "checkPhoneForSvcActivation": false,
        "pushUserId": "d1c05335246da68c1846408382864fbb9d5dc0987c23e97c1559a994a6baa5c8",
        "subscriber": {
            "accountId": null,
            "id": 3147327,
            "name": "Пользователь",
            "nickName": null
        },
        "subscriberPhones": [
            {
                "id": 2821552,
                "number": "79213012492",
                "numberValid": true
            }
        ]
    }
}
*/

type Subscriber struct {
	AccountId *string `json:"accountId"`
	Id        int     `json:"id"`
	Name      string  `json:"name"`
	NickName  *string `json:"nickName"`
}

type SubscriberPhone struct {
	Id          int    `json:"id"`
	Number      string `json:"number"`
	NumberValid bool   `json:"numberValid"`
}

type SubscriberProfilesResponse struct {
	AllowAddPhone              bool              `json:"allowAddPhone"`
	CallSelectedPlaceOnly      bool              `json:"callSelectedPlaceOnly"`
	CheckPhoneForSvcActivation bool              `json:"checkPhoneForSvcActivation"`
	PushUserId                 string            `json:"pushUserId"`
	Subscriber                 Subscriber        `json:"subscriber"`
	SubscriberPhones           []SubscriberPhone `json:"subscriberPhones"`
}
