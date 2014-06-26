package servertest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	"bitbucket.org/kardianos/osext"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("POST /users", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		params  *url.Values
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		root, err := osext.ExecutableFolder()
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer(root, db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	doRequest := func() {
		var err error
		req, err = NewFormRequest("POST", "/users", params)
		Expect(err).To(BeNil())
		handler.ServeHTTP(res, req)
	}

	Context("when valid params are received", func() {
		BeforeEach(func() {
			params = &url.Values{}
			params.Set("email", "foo@example.com")
			params.Set("username", "foo")
			params.Set("password", "123456")
			doRequest()
		})

		It("responds with ok", func() {
			Expect(res.Code).To(Equal(http.StatusOK))
			status := struct {
				Status string `json:"status"`
			}{}
			err := json.Unmarshal(res.Body.Bytes(), &status)
			Expect(err).To(BeNil())
			Expect(status.Status).To(Equal("ok"))
		})

		It("creates a user record in the db", func() {
			u, err := user.FindByUsername(db, "foo")
			Expect(err).To(BeNil())
			Expect(u).NotTo(BeNil())
			Expect(u.Email).To(Equal("foo@example.com"))
			Expect(u.Username).To(Equal("foo"))
			Expect(u.CreatedAt).NotTo(BeNil())
		})

		It("encrypts the password using bcrypt", func() {
			u, err := user.FindByUsername(db, "foo")
			Expect(err).To(BeNil())
			Expect(u).NotTo(BeNil())
			Expect(bcrypt.Verify(u.EncryptedPassword, "123456")).To(BeTrue())
		})
	})
})
