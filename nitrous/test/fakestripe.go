package test

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/stripe/stripe-go"
)

type FakeStripe struct {
	ReceivedRequests []map[string]string
	Responses        []string
	ResponseIndex    int
	Error            *stripe.Error
}

func (s *FakeStripe) Call(method, path, key string, body *url.Values, v interface{}) error {
	m := map[string]string{
		"method": method,
		"path":   path,
	}
	if body != nil {
		m["card"] = body.Get("card")
		m["email"] = body.Get("email")
		m["desc"] = body.Get("description")
		if s := body.Get("amount"); s != "" {
			m["amount"] = s
		}
		if s := body.Get("customer"); s != "" {
			m["customer"] = s
		}
		for k, v := range *body {
			if len(v) == 0 {
				continue
			}
			if strings.HasPrefix(k, "metadata[") {
				m[k] = v[0]
			}
		}
	}
	s.ReceivedRequests = append(s.ReceivedRequests, m)

	if s.Error != nil {
		return s.Error
	}

	if len(s.Responses) > s.ResponseIndex && s.Responses[s.ResponseIndex] != "" {
		indx := s.ResponseIndex
		s.ResponseIndex += 1

		// stripe may pass in a nil interface
		if v == nil {
			return nil
		}
		res := json.Unmarshal([]byte(s.Responses[indx]), v)
		return res
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

var StripeCardDeclined = &stripe.Error{
	Type: stripe.ErrorType(stripe.CardErr),
	Msg:  "card declined",
	Code: stripe.ErrorCode(stripe.CardDeclined),
}

var StripeCardExpired = &stripe.Error{
	Type: stripe.ErrorType(stripe.CardErr),
	Msg:  "card expired",
	Code: stripe.ErrorCode(stripe.ExpiredCard),
}

var StripeIncorrectCvc = &stripe.Error{
	Type: stripe.ErrorType(stripe.CardErr),
	Msg:  "incorrect cvc",
	Code: stripe.ErrorCode(stripe.IncorrectCvc),
}

var StripeIncorrectZip = &stripe.Error{
	Type: stripe.ErrorType(stripe.CardErr),
	Msg:  "incorrect zip",
	Code: stripe.ErrorCode(stripe.IncorrectZip),
}
