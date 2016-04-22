package test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	stripe "github.com/stripe/stripe-go"
)

type FakeStripe struct {
	ReceivedRequests []map[string]string
	Responses        []string
	RequestIndex     int
	Errors           []*stripe.Error
}

func (s *FakeStripe) Call(method, path, key string, body *url.Values, params *stripe.Params, v interface{}) error {
	fmt.Println("hello", key)
	m := map[string]string{
		"method": method,
		"path":   path,
	}
	if body != nil {
		m["card"] = body.Get("card")
		m["email"] = body.Get("email")
		m["desc"] = body.Get("description")

		optionalKeys := []string{"amount", "customer",
			"subscription", "subscription_plan", "plan", "coupon",
			"at_period_end", "trial_end"}
		for _, k := range optionalKeys {
			if s := body.Get(k); s != "" {
				m[k] = s
			}
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

	indx := s.RequestIndex
	s.RequestIndex += 1

	if len(s.Errors) > indx {
		if e := s.Errors[indx]; e != nil {
			return e
		}
	}

	if len(s.Responses) > indx && s.Responses[indx] != "" {
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

func (s *FakeStripe) CallMultipart(method, path, key, boundary string, body io.Reader, params *stripe.Params, v interface{}) error {
	return nil
}

// From https://stripe.com/docs/api#customers
var StripeCustomerResponse = `{
	"id": "sc_01",
	"object": "customer",
	"account_balance": 0,
	"created": 1459540945,
	"currency": "usd",
	"default_source": "card_17vYTMK23cit468SqKvPooOo",
	"delinquent": false,
	"description": "Name: , Username: card17",
	"discount": null,
	"email": "card17@example.com",
	"livemode": false,
	"metadata": {
		"username": "card17"
	},
	"shipping": null,
	"sources": {
		"object": "list",
		"data": [
			{
				"id": "card_17vYTMK23cit468SqKvPooOo",
				"object": "card",
				"address_city": null,
				"address_country": null,
				"address_line1": null,
				"address_line1_check": null,
				"address_line2": null,
				"address_state": null,
				"address_zip": "123123",
				"address_zip_check": "pass",
				"brand": "Visa",
				"country": "US",
				"customer": "cus_8BwMPDL426wNnn",
				"cvc_check": "pass",
				"dynamic_last4": null,
				"exp_month": 10,
				"exp_year": 2018,
				"fingerprint": "H0SJieNrXmTdq3qs",
				"funding": "credit",
				"last4": "4242",
				"metadata": {
				},
				"name": "gru",
				"tokenization_method": null
			}
		],
		"has_more": false,
		"total_count": 1,
		"url": "/v1/customers/cus_8BwMPDL426wNnn/sources"
	},
	"subscriptions": {
	"object": "list",
	"data": [
	],
	"has_more": false,
	"total_count": 0,
	"url": "/v1/customers/cus_8BwMPDL426wNnn/subscriptions"
	}
}`

// from api docs https://stripe.com/docs/api/go#list_invoices
var StripeInvoiceResponse = `{
  "id": "in_17AYBv2eZvKYlo2C8sW8Cs4R",
  "object": "invoice",
  "amount_due": 3000,
  "application_fee": null,
  "attempt_count": 0,
  "attempted": false,
  "charge": null,
  "closed": false,
  "currency": "usd",
  "customer": "cus_7PN4EHPJGbC4D1",
  "date": 1448338447,
  "description": null,
  "discount": null,
  "ending_balance": null,
  "forgiven": false,
  "lines": {
    "data": [
      {
        "id": "sub_7PNezyXAgzNnkn",
        "object": "line_item",
        "amount": 2000,
        "currency": "usd",
        "description": null,
        "discountable": true,
        "livemode": true,
        "metadata": {
        },
        "period": {
          "start": 1450933120,
          "end": 1453611520
        },
        "plan": {
          "id": "gold21323",
          "object": "plan",
          "amount": 2000,
          "created": 1386247539,
          "currency": "usd",
          "interval": "month",
          "interval_count": 1,
          "livemode": false,
          "metadata": {
          },
          "name": "New plan name",
          "statement_descriptor": null,
          "trial_period_days": null
        },
        "proration": false,
        "quantity": 1,
        "subscription": null,
        "type": "subscription"
      }
    ],
    "total_count": 1,
    "object": "list",
    "url": "/v1/invoices/in_17AYBv2eZvKYlo2C8sW8Cs4R/lines"
  },
  "livemode": false,
  "metadata": {
  },
  "next_payment_attempt": 1448342047,
  "paid": false,
  "period_end": 1448338371,
  "period_start": 1448251971,
  "receipt_number": null,
  "starting_balance": 0,
  "statement_descriptor": null,
  "subscription": "sub_4gmRwSQmlmUOs8",
  "subtotal": 3000,
  "tax": null,
  "tax_percent": null,
  "total": 3000,
  "webhooks_delivered_at": 1448338461
}`

var StripeInvoiceListResponse = fmt.Sprintf(`{"data": [%s]}`, StripeInvoiceResponse)

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
