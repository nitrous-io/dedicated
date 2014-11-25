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
