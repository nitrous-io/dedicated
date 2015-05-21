package freequeue_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	cfq "github.com/nitrous-io/dedicated/nitrous/controllers/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/dns"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/freequeue")
}

var _ = Describe("controllers/freequeue", func() {
	var (
		db      *db.Database
		handler *mux.Router

		u  *user.User
		t  *token.Token
		ft *test.FakeTracker
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		ft = test.NewFakeTracker()
		svr := server.NewServer("", db, logger, nil, &dns.FakeProvider{})
		svr.Tracker = ft

		handler = svr.Router()

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Name:     "foo",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/free_queue", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/free_queue", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should return an error if there is no place in the freequeue", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errorSlug, errorDesc := test.ErrorSlug(res.Body)
			Expect(errorSlug).To(Equal("not_in_queue"))
			Expect(errorDesc).To(Equal("you do not have a place in the queue"))
		})

		It("should return an error if there if a freequeue record was soft deleted", func() {
			fq := &freequeue.FreeQueue{
				UserID:                u.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				RegionSlug:            "slug1",
				HostName:              "name1",
			}
			Expect(fq.Insert(db)).To(BeNil())

			_, err := db.Exec("UPDATE free_queue SET deleted_at = now() WHERE id = $1", fq.ID)
			Expect(err).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errorSlug, errorDesc := test.ErrorSlug(res.Body)
			Expect(errorSlug).To(Equal("not_in_queue"))
			Expect(errorDesc).To(Equal("you do not have a place in the queue"))
		})

		Context("with a free queue", func() {
			var fq *freequeue.FreeQueue

			BeforeEach(func() {
				fq = &freequeue.FreeQueue{
					UserID:                u.ID,
					ContainerTemplateSlug: "tmpl1",
					ContainerName:         "nm",
					RegionSlug:            "slug1",
					HostName:              "name1",
				}
				Expect(fq.Insert(db)).To(BeNil())
			})

			It("should return an error if free queue is found but a host has previously been created", func() {
				_, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-host-1",
						HourlyPrice: 2000,
					},
					U: u,
				})
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("queue_used"))
				Expect(errorDesc).To(Equal("you have already used your host allocation"))
			})

			It("should return an error if free queue is found but a host has previously been terminated", func() {
				hs, err := test.CreateHostStack(db, &test.HostStack{
					H: &host.Host{
						Slug:        "my-host-1",
						HourlyPrice: 2000,
					},
					U: u,
				})
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE hosts SET terminated_at = now(), updated_at = now() WHERE id = $1;", hs.H.ID)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusForbidden))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("queue_used"))
				Expect(errorDesc).To(Equal("you have already used your host allocation"))
			})

			It("should serialize the response with the template", func() {
				t := &template.Template{
					Slug:        "tmpl1",
					Name:        sql.NullString{"Ruby on Rails", true},
					Description: sql.NullString{"A Rails stack template", true},
					Image:       "rails:4.2",
					Type:        template.TypeStack,
					Config:      `{ "ports": ["private:5432:5432", "public:2222:22"], "publish_all": true }`,
				}
				err := t.Insert(db)
				Expect(err).To(BeNil())

				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					FreeQueue *serializers.FreeQueue `json:"free_queue"`
					Template  *serializers.Template  `json:"template"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.FreeQueue.ID).To(Equal(fq.ID))
				Expect(j.Template.ID).To(Equal(t.ID))
			})

		})
	})

	Describe("DELETE /api/v0/free_queue", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "DELETE", "/api/v0/free_queue", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should perform a delete", func() {
			fq := &freequeue.FreeQueue{
				UserID:                u.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				RegionSlug:            "slug1",
				HostName:              "name1",
			}
			Expect(fq.Insert(db)).To(BeNil())

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			_, err := freequeue.Find(db, u.ID)
			Expect(err).To(Equal(sql.ErrNoRows))

			// and track on mixpanel
			Expect(ft.PropsCalled).To(HaveLen(1))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.PropsSet["waitlist_canceled_date"]).To(BeTemporally("~", time.Now(), time.Minute))
		})
	})

	Describe("GET /api/v0/admin/free_queue/pending", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := test.NewAuthedRequest(t, "GET", "/api/v0/admin/free_queue/pending", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should return a 404 if requester is not an admin", func() {
			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		Context("when the requester is an admin", func() {
			var (
				fq1, fq2, fq3 *freequeue.FreeQueue
			)

			BeforeEach(func() {
				_, err := db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				u1 := &user.User{
					Username: "bar",
					Email:    "bar@example.com",
					Password: "123456",
				}
				err = test.WrapInTransaction(db, u1.Insert)
				Expect(err).To(BeNil())

				u2 := &user.User{
					Username: "baz",
					Email:    "baz@example.com",
					Password: "123456",
				}
				err = test.WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				u3 := &user.User{
					Username: "bob",
					Email:    "bob@example.com",
					Password: "123456",
				}
				err = test.WrapInTransaction(db, u3.Insert)
				Expect(err).To(BeNil())

				fq1 = &freequeue.FreeQueue{
					UserID:                u1.ID,
					ContainerTemplateSlug: "tmpl1",
					ContainerName:         "nm",
					RegionSlug:            "slug1",
				}
				Expect(fq1.Insert(db)).To(BeNil())

				fq2 = &freequeue.FreeQueue{
					UserID:                u2.ID,
					ContainerTemplateSlug: "tmpl1",
					ContainerName:         "nm",
					RegionSlug:            "slug1",
				}
				Expect(fq2.Insert(db)).To(BeNil())

				fq3 = &freequeue.FreeQueue{
					UserID:                u3.ID,
					ContainerTemplateSlug: "tmpl1",
					ContainerName:         "nm",
					RegionSlug:            "slug1",
				}
				Expect(fq3.Insert(db)).To(BeNil())

				_, err = db.Exec("UPDATE free_queue SET state = 'approved' WHERE id = $1", fq3.ID)
				Expect(err).To(BeNil())
			})

			It("should return a list of pending users", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Queue []*serializers.FreeQueueWithUser `json:"queue"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Queue).To(HaveLen(2))
				Expect([]int64{j.Queue[0].ID, j.Queue[1].ID}).To(ConsistOf([]int64{fq1.ID, fq2.ID}))
			})
		})
	})

	Describe("POST /api/v0/admin/free_queue/:id/action", func() {
		var (
			fq *freequeue.FreeQueue

			fakeMailer *test.FakeMailer
			origMailer mailer.Mailer
		)

		BeforeEach(func() {
			u1 := &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			fq = &freequeue.FreeQueue{
				UserID:                u1.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				RegionSlug:            "region1",
			}
			Expect(fq.Insert(db)).To(BeNil())

			envvars.WebHostname = "nitrous.example"
			origMailer = user.Mailer
			fakeMailer = &test.FakeMailer{}
			cfq.Mailer = fakeMailer
		})

		AfterEach(func() {
			cfq.Mailer = origMailer
		})

		doRequest := func(id int64, params *url.Values) *httptest.ResponseRecorder {
			uri := fmt.Sprintf("/api/v0/admin/free_queue/%d/action", id)
			req, err := test.NewAuthedRequest(t, "POST", uri, params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should return a 404 if requester is not an admin", func() {
			res := doRequest(fq.ID, nil)
			Expect(res.Code).To(Equal(http.StatusNotFound))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		Context("when the requester is an admin", func() {
			BeforeEach(func() {
				_, err := db.Exec(`UPDATE users SET admin=true WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should require that expected_state is sent", func() {
				res := doRequest(fq.ID, nil)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("invalid_request"))
				Expect(errorDesc).To(Equal("`expected_state` is required"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should require that new_state is sent", func() {
				params := &url.Values{"expected_state": []string{fq.State}}
				res := doRequest(fq.ID, params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("invalid_request"))
				Expect(errorDesc).To(Equal("`new_state` is required"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should return an error if the expected state does not match the current state", func() {
				params := &url.Values{"expected_state": []string{"approved"}, "new_state": []string{"rejected"}}
				res := doRequest(fq.ID, params)
				Expect(res.Code).To(Equal(http.StatusConflict))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("inconsistent_state"))
				Expect(errorDesc).To(Equal("the provided `expected_state` does not match the current state"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should return an error if the new state is invalid", func() {
				params := &url.Values{"expected_state": []string{fq.State}, "new_state": []string{"apples"}}
				res := doRequest(fq.ID, params)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errorSlug, errorDesc := test.ErrorSlug(res.Body)
				Expect(errorSlug).To(Equal("invalid_request"))
				Expect(errorDesc).To(Equal("the provided `new_state` is invalid"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should set the state on the record", func() {
				params := &url.Values{"expected_state": []string{fq.State}, "new_state": []string{"rejected"}}
				res := doRequest(fq.ID, params)
				Expect(res.Code).To(Equal(http.StatusOK))

				j := make(map[string]interface{})
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j["status"]).To(Equal("ok"))

				err = db.Get(fq, "SELECT * FROM free_queue WHERE id = $1", fq.ID)
				Expect(err).To(BeNil())
				Expect(fq.State).To(Equal(freequeue.StateRejected))
				Expect(fq.ActionedBy).To(Equal(sql.NullInt64{u.ID, true}))
				Expect(fq.ActionedAt.Valid).To(BeTrue())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())

				// and track on mixpanel
				Expect(ft.PropsCalled).To(HaveLen(1))
				pc := ft.PropsCalled[0]
				Expect(pc.UserID).To(Equal(fq.UserID))
				Expect(pc.Mode).To(Equal("set"))
				Expect(pc.PropsSet["waitlist_rejected_date"]).To(BeTemporally("~", time.Now(), time.Minute))
			})

			It("should send off the email if approved", func() {
				params := &url.Values{"expected_state": []string{fq.State}, "new_state": []string{"approved"}}
				res := doRequest(fq.ID, params)
				Expect(res.Code).To(Equal(http.StatusOK))

				j := make(map[string]interface{})
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j["status"]).To(Equal("ok"))

				Expect(fakeMailer.SendMailCalled).To(BeTrue())
				Expect(fakeMailer.From).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
				Expect(fakeMailer.Tos).To(Equal([]string{"\"bar\" <bar@example.com>"}))
				Expect(fakeMailer.ReplyTo).To(Equal("\"Nitrous Pro\" <mailer@nitrous.io>"))
				Expect(fakeMailer.Subject).To(Equal("Your Nitrous workspace is ready!"))
				Expect(fakeMailer.Body).To(ContainSubstring("Great news bar!"))
				Expect(fakeMailer.Body).To(ContainSubstring("https://nitrous.example/app/#/free-tier"))
				Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Great news bar!</h3>"))
				Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/free-tier" target="_blank`))

				// and track on mixpanel
				Expect(ft.PropsCalled).To(HaveLen(1))
				pc := ft.PropsCalled[0]
				Expect(pc.UserID).To(Equal(fq.UserID))
				Expect(pc.Mode).To(Equal("set"))
				Expect(pc.PropsSet["waitlist_approved_date"]).To(BeTemporally("~", time.Now(), time.Minute))
			})
		})
	})

})
