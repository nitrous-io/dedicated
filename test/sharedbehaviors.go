package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type AuthTokenInputs struct {
	Handler http.Handler
	Path    string
	Method  string
}

func ItRequiresAuthToken(inputs *AuthTokenInputs) {
	var (
		res *httptest.ResponseRecorder
	)

	doRequest := func(token string) {
		req, err := NewRequest(inputs.Method, inputs.Path, nil)
		Expect(err).To(BeNil())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res = httptest.NewRecorder()
		inputs.Handler.ServeHTTP(res, req)
	}

	Context("when an invalid oauth token is given", func() {
		BeforeEach(func() {
			doRequest("invalidtoken")
		})

		It("responds with 401 Unauthorized", func() {
			Expect(res.Code).To(Equal(http.StatusUnauthorized))
			Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_token"`))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("invalid_token"))
		})
	})

	Context("when no oauth token is given", func() {
		BeforeEach(func() {
			doRequest("")
		})

		It("responds with 400 Bad Request", func() {
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_request"`))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("invalid_request"))
		})
	})
}
