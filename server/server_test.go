package server

import (
	"encoding/json"
	"github.com/gorilla/mux"
	. "github.com/motain/gocheck"
	. "github.com/nitrous-io/freighter/testhelper"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type FreighterdTestSuite struct {
	handler *mux.Router
	res     *httptest.ResponseRecorder
}

var _ = Suite(&FreighterdTestSuite{})

func (s *FreighterdTestSuite) SetUpTest(c *C) {
	s.handler = NewRouter()
	s.res = httptest.NewRecorder()
}

func (s *FreighterdTestSuite) TestHome(c *C) {
	req, err := NewRequest("GET", "/", nil)
	c.Assert(err, IsNil)

	s.handler.ServeHTTP(s.res, req)

	c.Assert(s.res.Code, Equals, http.StatusOK)

	var j map[string]interface{}
	err = json.Unmarshal(s.res.Body.Bytes(), &j)
	c.Assert(err, IsNil)

	c.Assert(j["status"], Equals, "ok")
}
