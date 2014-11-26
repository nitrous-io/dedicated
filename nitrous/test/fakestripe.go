package test

import (
	"encoding/json"
	"net/url"

	"github.com/stripe/stripe-go"
)

type FakeStripe struct {
	Received map[string]string
	Response string
	Error    *stripe.Error
}

func (s *FakeStripe) Call(method, path, key string, body *url.Values, v interface{}) error {
	s.Received = map[string]string{
		"method": method,
		"path":   path,
		"card":   body.Get("card"),
		"email":  body.Get("email"),
		"desc":   body.Get("description"),
	}

	if s.Error != nil {
		return s.Error
	}

	if s.Response != "" {
		return json.Unmarshal([]byte(s.Response), v)
	}

	return &stripe.Error{
		Type: stripe.ErrorType("test without response or proper error"),
	}
}

var StripeCustomerResponse = `{
		"id": "sc_01",
		"email": "user@example.com",
		"cards": {
			"object": "list",
			"total_count": 1,
			"has_more": false,
			"url": "/v1/customers/cus_56dgsyZAJujFFO/cards",
			"data": [
				{
					"id": "card_14wQPX2CU2PklWtHi34KFonT",
					"object": "card",
					"last4": "4242",
					"brand": "Visa",
					"funding": "credit",
					"exp_month": 10,
					"exp_year": 2018,
					"fingerprint": "H0SJieNrXmTdq3qs",
					"country": "US",
					"name": null,
					"address_line1": null,
					"address_line2": null,
					"address_city": null,
					"address_state": null,
					"address_zip": null,
					"address_country": null,
					"cvc_check": "pass",
					"address_line1_check": null,
					"address_zip_check": null,
					"dynamic_last4": null,
					"customer": "cus_56dgsyZAJujFFO"
				}
			]
		},
		"default_card": "card_14wQPX2CU2PklWtHi34KFonT"
	}`
