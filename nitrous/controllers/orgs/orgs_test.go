package orgs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/orgs")
}

var _ = Describe("controllers/orgs", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		params  *url.Values

		authTokenInputs test.AuthTokenInputs
		u               *user.User
		t               *token.Token
		ft              *test.FakeTracker
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
		svr.Tracker = ft
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
			Name:     "Mr. Foo",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

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

	Describe("POST /api/v0/orgs", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/orgs", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("email", "Accounting@Nitrous.io")
			params.Set("name", "Nitrous, Inc.")
			params.Set("slug", "nitRous")
		})

		Context("when valid params are received", func() {
			BeforeEach(func() {
				doRequest()
			})

			It("returns 201 Created and creates an org in the db", func() {
				Expect(res.Code).To(Equal(http.StatusCreated))
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())
				Expect(o).NotTo(BeNil())
				Expect(o.ID).NotTo(BeZero())
				Expect(o.Email).To(Equal("accounting@nitrous.io"))
				Expect(o.Name).To(Equal("Nitrous, Inc."))
				Expect(o.Slug).To(Equal("nitrous"))
				Expect(o.CreatedAt).NotTo(BeNil())
				Expect(o.DeletedAt.Valid).To(BeFalse())
			})

			It("responds with a json representation of the org that's been created", func() {
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())

				Expect(len(res.Body.Bytes())).NotTo(BeZero())
				var j struct {
					Org *serializers.Org `json:"org"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Org.ID).To(Equal(o.ID))
				Expect(j.Org.Email).To(Equal("accounting@nitrous.io"))
				Expect(j.Org.Name).To(Equal("Nitrous, Inc."))
				Expect(j.Org.Slug).To(Equal("nitrous"))
			})

			It("adds the user as the owner of the org", func() {
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())

				Expect(m).To(HaveLen(1))
				Expect(m[0].Username).To(Equal("foo"))
				Expect(m[0].Role).To(Equal("owner"))
			})
		})

		Context("when invalid params are received", func() {
			type tc struct {
				param   string
				value   string
				message string
			}
			tcs := []tc{
				{"slug", "", org.ErrSlugBlank.Error()},
				{"slug", "!@$@!$", org.ErrSlugInvalid.Error()},
				{"email", "", org.ErrEmailBlank.Error()},
				{"email", "not.a.valid.email@", org.ErrEmailInvalid.Error()},
				{"email", "hello@yopmail.com", org.ErrEmailBlacklisted.Error()},
				{"name", "", org.ErrNameBlank.Error()},
			}

			for _, t := range tcs {
				func(t tc) {
					Context(fmt.Sprintf(`when "%s" param is "%s"`, t.param, t.value), func() {
						BeforeEach(func() {
							if t.value == "" {
								params.Del(t.param)
							} else {
								params.Set(t.param, t.value)
							}
							doRequest()
						})

						It("responds with 400 bad request", func() {
							Expect(res.Code).To(Equal(http.StatusBadRequest))
							var j map[string]string
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())
							Expect(j["error"]).To(Equal("validation_failed"))
							Expect(j["error_description"]).To(Equal(t.message))
						})
					})
				}(t)
			}
		})

		Context("when a banned email address is provided", func() {
			BeforeEach(func() {
				b := &user.User{
					Username: "banner",
					Password: "123456",
					Email:    "banner@example.com",
				}
				err := test.WrapInTransaction(db, b.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec("INSERT INTO users (username, email, encrypted_password, deleted_at, banned_at, banned_by_id) VALUES ('foo', 'foo@example.com', '!@#$%', now(), now(), $1)", b.ID)
				Expect(err).To(BeNil())

				params.Set("email", "foo@example.com")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				var j map[string]string
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("email_banned"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	FDescribe("PUT /api/v0/orgs/{slug}/members/{username}", func() {
		var (
			o, o2 *org.Org
			u2    *user.User
		)

		BeforeEach(func() {
			var err error

			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err = o.Insert(db)
			Expect(err).To(BeNil())

			o2 = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			err = o2.Insert(db)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
				Name:     "Ms. Bar",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			params = &url.Values{}
			params.Set("role", "member")
		})

		doRequest := func(slug, username string) {
			var err error
			url := fmt.Sprintf("/api/v0/orgs/%s/members/%s", slug, username)
			req, err = test.NewFormRequest("PUT", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when a non-existent org slug is given", func() {
			It("responds with 400 not found", func() {
				doRequest("pineapple-inc", u2.Username)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))
			})
		})

		Context("when a non-existent username is given", func() {
			It("responds with 404 not found", func() {
				doRequest(o.Slug, "baz")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))
			})
		})

		Context("when target user is same as requesting user", func() {
			It("responds with 400 bad request", func() {
				doRequest(o.Slug, u.Username)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_action"))
				Expect(j["error_description"]).To(Equal("you cannot edit yourself"))
			})
		})

		Context("when an invalid role is given", func() {
			It("responds with 400 bad request", func() {
				params.Set("role", "destroyer")
				doRequest(o.Slug, u2.Username)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				j := map[string]string{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_role"))
				Expect(j["error_description"]).To(Equal("destroyer is not a valid role"))
			})
		})

		Context("when user is not a part of the org", func() {
			It("responds with 403 forbidden", func() {
				doRequest(o.Slug, u2.Username)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				m, err = orgmembership.FindAllByOrgID(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(m).To(HaveLen(0))

				j := map[string]string{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("permission_denied"))
			})
		})

		Context("when a given user is already a member", func() {
			type tc struct {
				myRole  string
				from    string
				to      string
				allowed bool
			}
			tcs := []tc{
				{"owner", "admin", "owner", true}, // i am an owner, promoting an admin to owner
				{"owner", "admin", "admin", true},
				{"owner", "admin", "member", true}, // i am an owner, demoting an admin to member
				{"owner", "owner", "owner", true},
				{"owner", "owner", "admin", true},
				{"owner", "owner", "member", true},
				{"owner", "member", "owner", true},
				{"owner", "member", "admin", true},
				{"owner", "member", "member", true},

				{"admin", "admin", "owner", false},
				{"admin", "admin", "admin", true},
				{"admin", "admin", "member", true},
				{"admin", "owner", "owner", false},
				{"admin", "owner", "admin", false},
				{"admin", "owner", "member", false},
				{"admin", "member", "owner", false},
				{"admin", "member", "admin", true},
				{"admin", "member", "member", true},

				{"member", "admin", "owner", false},
				{"member", "admin", "admin", false},
				{"member", "admin", "member", false},
				{"member", "owner", "owner", false},
				{"member", "owner", "admin", false},
				{"member", "owner", "member", false},
				{"member", "member", "owner", false},
				{"member", "member", "admin", false},
				{"member", "member", "member", false},
			}
			for _, t := range tcs {
				func(t tc) {
					Context(fmt.Sprintf("when %s requests update of a member's role from %s to %s", t.myRole, t.from, t.to), func() {
						BeforeEach(func() {
							err := orgmembership.AddMember(db, o.ID, u.ID, t.myRole)
							Expect(err).To(BeNil())
							err = orgmembership.AddMember(db, o.ID, u2.ID, t.from)
							Expect(err).To(BeNil())

							params.Set("role", t.to)
							doRequest(o.Slug, u2.Username)
						})

						if t.allowed {
							It("allows update", func() {
								Expect(res.Code).To(Equal(http.StatusOK))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(2))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))
								Expect(m[1].Username).To(Equal(u2.Username))
								// role should have been updated
								Expect(m[1].Role).To(Equal(t.to))

								m, err = orgmembership.FindAllByOrgID(db, o2.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(0))

								var j struct {
									Members []*serializers.OrgMember `json:"members"`
								}
								err = json.Unmarshal(res.Body.Bytes(), &j)
								Expect(err).To(BeNil())
								Expect(j.Members).To(HaveLen(2))
								Expect(j.Members[0].UserID).To(Equal(u.ID))
								Expect(j.Members[0].Username).To(Equal(u.Username))
								Expect(j.Members[0].Email).To(Equal(u.Email))
								Expect(j.Members[0].Role).To(Equal(t.myRole))
								Expect(j.Members[0].Name).To(Equal(u.Name))
								Expect(j.Members[1].UserID).To(Equal(u2.ID))
								Expect(j.Members[1].Username).To(Equal(u2.Username))
								Expect(j.Members[1].Email).To(Equal(u2.Email))
								Expect(j.Members[1].Role).To(Equal(t.to))
								Expect(j.Members[1].Name).To(Equal(u2.Name))
							})
						} else {
							It("disallows update", func() {
								Expect(res.Code).To(Equal(http.StatusForbidden))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(2))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))
								Expect(m[1].Username).To(Equal(u2.Username))
								// role shouldn't have been updated
								Expect(m[1].Role).To(Equal(t.from))

								m, err = orgmembership.FindAllByOrgID(db, o2.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(0))
							})
						}
					})
				}(t)
			}
		})

		Context("when a given user is not yet a member", func() {
			type tc struct {
				myRole  string
				role    string
				allowed bool
			}
			tcs := []tc{
				{"owner", "owner", true}, // i am an owner, adding an owner
				{"owner", "admin", true}, // i am an owner, adding an admin
				{"owner", "member", true},
				{"admin", "owner", false},
				{"admin", "admin", true},
				{"admin", "member", true},
				{"member", "owner", false},
				{"member", "admin", false},
				{"member", "member", false},
			}
			for _, t := range tcs {
				func(t tc) {
					Context(fmt.Sprintf("when %s requests a new %s to be added", t.myRole, t.role), func() {
						BeforeEach(func() {
							err := orgmembership.AddMember(db, o.ID, u.ID, t.myRole)
							Expect(err).To(BeNil())

							params.Set("role", t.role)
							doRequest(o.Slug, u2.Username)
						})

						if t.allowed {
							It("allows addition", func() {
								Expect(res.Code).To(Equal(http.StatusCreated))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(2))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))
								// new org member should have been added with correct role
								Expect(m[1].Username).To(Equal(u2.Username))
								Expect(m[1].Role).To(Equal(t.role))

								m, err = orgmembership.FindAllByOrgID(db, o2.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(0))

								var j struct {
									Members []*serializers.OrgMember `json:"members"`
								}
								err = json.Unmarshal(res.Body.Bytes(), &j)
								Expect(err).To(BeNil())
								Expect(j.Members).To(HaveLen(2))
								Expect(j.Members[0].UserID).To(Equal(u.ID))
								Expect(j.Members[0].Username).To(Equal(u.Username))
								Expect(j.Members[0].Email).To(Equal(u.Email))
								Expect(j.Members[0].Role).To(Equal(t.myRole))
								Expect(j.Members[0].Name).To(Equal(u.Name))
								Expect(j.Members[1].UserID).To(Equal(u2.ID))
								Expect(j.Members[1].Username).To(Equal(u2.Username))
								Expect(j.Members[1].Email).To(Equal(u2.Email))
								Expect(j.Members[1].Role).To(Equal(t.role))
								Expect(j.Members[1].Name).To(Equal(u2.Name))
							})
						} else {
							It("disallows addition", func() {
								Expect(res.Code).To(Equal(http.StatusForbidden))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								// new org member shouldn't have been added
								Expect(m).To(HaveLen(1))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))

								m, err = orgmembership.FindAllByOrgID(db, o2.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(0))
							})
						}
					})
				}(t)
			}
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs/foobar/members/foo"
				authTokenInputs.Method = "PUT"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/orgs/{slug}/members/{username}", func() {
		var (
			o, o2 *org.Org
			u2    *user.User
		)

		BeforeEach(func() {
			var err error

			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err = o.Insert(db)
			Expect(err).To(BeNil())

			o2 = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			err = o2.Insert(db)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
				Name:     "Ms. Bar",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())
		})

		doRequest := func(slug, username string) {
			var err error
			url := fmt.Sprintf("/api/v0/orgs/%s/members/%s", slug, username)
			req, err = test.NewFormRequest("DELETE", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when an org with a given slug does not exist", func() {
			It("responds with 404 not found", func() {
				doRequest("pineapple-inc", u2.Username)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when requesting user does not belong to the org", func() {
			It("responds with 403 forbidden", func() {
				doRequest(o.Slug, u2.Username)
				Expect(res.Code).To(Equal(http.StatusForbidden))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("permission_denied"))
			})
		})

		Context("when target user does not belong to the org", func() {
			It("responds with 404 not found", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "admin")
				Expect(err).To(BeNil())
				err = orgmembership.AddMember(db, o2.ID, u2.ID, "member")
				Expect(err).To(BeNil())

				doRequest(o.Slug, u2.Username)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when target user is same as requesting user", func() {
			It("responds with 400 bad request", func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "admin")
				Expect(err).To(BeNil())

				doRequest(o.Slug, u.Username)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				j := map[string]string{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_action"))
				Expect(j["error_description"]).To(Equal("you cannot remove yourself"))
			})
		})

		Context("when requesting user belongs to the org", func() {
			type tc struct {
				myRole         string
				targetUserRole string
				allowed        bool
			}
			tcs := []tc{
				{"owner", "owner", true},
				{"owner", "admin", true},
				{"owner", "member", true},
				{"admin", "owner", false},
				{"admin", "admin", true},
				{"admin", "member", true},
				{"member", "owner", false},
				{"member", "admin", false},
				{"member", "member", false},
			}
			for _, t := range tcs {
				func(t tc) {
					Context(fmt.Sprintf("when %s requests deletion of %s", t.myRole, t.targetUserRole), func() {
						BeforeEach(func() {
							err := orgmembership.AddMember(db, o.ID, u.ID, t.myRole)
							Expect(err).To(BeNil())
							err = orgmembership.AddMember(db, o.ID, u2.ID, t.targetUserRole)
							Expect(err).To(BeNil())

							doRequest(o.Slug, u2.Username)
						})

						if t.allowed {
							It("allows deletion", func() {
								Expect(res.Code).To(Equal(http.StatusOK))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(1))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))

								var j struct {
									Members []*serializers.OrgMember `json:"members"`
								}
								err = json.Unmarshal(res.Body.Bytes(), &j)
								Expect(err).To(BeNil())
								Expect(j.Members).To(HaveLen(1))
								Expect(j.Members[0].UserID).To(Equal(u.ID))
								Expect(j.Members[0].Username).To(Equal(u.Username))
								Expect(j.Members[0].Email).To(Equal(u.Email))
								Expect(j.Members[0].Role).To(Equal(t.myRole))
								Expect(j.Members[0].Name).To(Equal(u.Name))
							})
						} else {
							It("disallows deletion", func() {
								Expect(res.Code).To(Equal(http.StatusForbidden))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(2))
								Expect(m[0].Username).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))
								Expect(m[1].Username).To(Equal(u2.Username))
								Expect(m[1].Role).To(Equal(t.targetUserRole))
							})
						}
					})
				}(t)
			}
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs/foobar/members/foo"
				authTokenInputs.Method = "DELETE"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/orgs/{slug}", func() {
		var (
			o, o2 *org.Org
		)

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())

			o2 = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			err = o2.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) {
			var err error
			url := fmt.Sprintf("/api/v0/orgs/%s", slug)
			req, err = test.NewRequest("GET", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when an org with a given slug does not exist", func() {
			It("responds with 404 not found", func() {
				doRequest("pineapple-inc")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when user does not belong to the org", func() {
			It("responds with 404 not found", func() {
				doRequest(o.Slug)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when user belongs to the org", func() {
			BeforeEach(func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "member")
				Expect(err).To(BeNil())
			})

			It("responds with 200 ok and org information", func() {
				doRequest(o.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Org *serializers.Org `json:"org"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Org.ID).To(Equal(o.ID))
				Expect(j.Org.Slug).To(Equal(o.Slug))
				Expect(j.Org.Name).To(Equal(o.Name))
				Expect(j.Org.Email).To(Equal(o.Email))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs/foobar"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/orgs/{slug}/members", func() {
		var (
			o, o2  *org.Org
			u2, u3 *user.User
		)

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())

			o2 = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			err = o2.Insert(db)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
				Name:     "Ms. Bar",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			u3 = &user.User{
				Username: "baz",
				Email:    "baz@example.com",
				Password: "123456",
				Name:     "Mrs. Baz",
			}
			err = test.WrapInTransaction(db, u3.Insert)
			Expect(err).To(BeNil())

			err = orgmembership.AddMember(db, o.ID, u.ID, "owner")
			Expect(err).To(BeNil())
			err = orgmembership.AddMember(db, o2.ID, u2.ID, "member")
			Expect(err).To(BeNil())
			err = orgmembership.AddMember(db, o.ID, u3.ID, "admin")
			Expect(err).To(BeNil())
		})

		doRequest := func(slug string) {
			var err error
			url := fmt.Sprintf("/api/v0/orgs/%s/members", slug)
			req, err = test.NewRequest("GET", url, params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when an org with a given slug does not exist", func() {
			It("responds with 404 not found", func() {
				doRequest("pineapple-inc")
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when user does not belong to the org", func() {
			It("responds with 404 not found", func() {
				doRequest(o2.Slug)
				Expect(res.Code).To(Equal(http.StatusNotFound))

				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("not_found"))
			})
		})

		Context("when user belongs to the org", func() {
			It("responds with 200 and members of the org", func() {
				doRequest(o.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Members []*serializers.OrgMember `json:"members"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Members).To(HaveLen(2))
				Expect(j.Members[0].UserID).To(Equal(u.ID))
				Expect(j.Members[0].Username).To(Equal(u.Username))
				Expect(j.Members[0].Email).To(Equal(u.Email))
				Expect(j.Members[0].Role).To(Equal("owner"))
				Expect(j.Members[0].Name).To(Equal(u.Name))
				Expect(j.Members[1].UserID).To(Equal(u3.ID))
				Expect(j.Members[1].Username).To(Equal(u3.Username))
				Expect(j.Members[1].Email).To(Equal(u3.Email))
				Expect(j.Members[1].Role).To(Equal("admin"))
				Expect(j.Members[1].Name).To(Equal(u3.Name))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs/foobar/members"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/orgs", func() {
		var (
			o, o2, o3 *org.Org
		)

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Foo Bar, Inc.",
				Slug:  "foobar",
				Email: "eng@foobar.com",
			}
			err := o.Insert(db)
			Expect(err).To(BeNil())

			o2 = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			err = o2.Insert(db)
			Expect(err).To(BeNil())

			o3 = &org.Org{
				Name:  "Watermelon Corp.",
				Slug:  "watermelon",
				Email: "admin@watermelon.co",
			}
			err = o3.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func() {
			var err error
			req, err = test.NewRequest("GET", "/api/v0/orgs", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when user does not belong to any org", func() {
			It("returns 200 ok with empty result", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Orgs []*serializers.Org `json:"orgs"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Orgs).To(HaveLen(0))
			})
		})

		Context("when user belongs to orgs", func() {
			BeforeEach(func() {
				err := orgmembership.AddMember(db, o.ID, u.ID, "member")
				Expect(err).To(BeNil())

				err = orgmembership.AddMember(db, o3.ID, u.ID, "member")
				Expect(err).To(BeNil())
			})

			It("returns 200 ok with user's orgs", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Orgs []*serializers.Org `json:"orgs"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Orgs).To(HaveLen(2))
				Expect(j.Orgs[0].ID).To(Equal(o.ID))
				Expect(j.Orgs[0].Email).To(Equal(o.Email))
				Expect(j.Orgs[0].Name).To(Equal(o.Name))
				Expect(j.Orgs[0].Slug).To(Equal(o.Slug))
				Expect(j.Orgs[1].ID).To(Equal(o3.ID))
				Expect(j.Orgs[1].Email).To(Equal(o3.Email))
				Expect(j.Orgs[1].Name).To(Equal(o3.Name))
				Expect(j.Orgs[1].Slug).To(Equal(o3.Slug))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/orgs"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
