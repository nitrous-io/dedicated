package test

import (
	"encoding/json"
	"net/url"

	"github.com/stripe/stripe-go"
)

type receivedValues struct {
	Method string
	Path   string
	Key    string
	Card   string // stripechanges token param to card
	Email  string
	Desc   string
}

type FakeStripe struct {
	Received *receivedValues
	Response string
	Error    *stripe.Error
}

func (s *FakeStripe) Call(method, path, key string, body *url.Values, v interface{}) error {
	s.Received = &receivedValues{
		Method: method,
		Path:   path,
		Key:    key,
		Card:   body.Get("card"),
		Email:  body.Get("email"),
		Desc:   body.Get("description"),
	}

	if s.Error != nil {
		return s.Error
	} else {
		return json.Unmarshal([]byte(s.Response), v)
	}
}
