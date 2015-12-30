package orgs_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/nitrous/controllers/orgs"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
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
		doRequest := func() *httptest.ResponseRecorder {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/orgs", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("email", "Accounting@Nitrous.io")
			params.Set("name", "Nitrous, Inc.")
			params.Set("slug", "nitRous")
		})

		Context("when valid params are received", func() {
			It("returns 201 Created and creates an org in the db", func() {
				res := doRequest()
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
				Expect(o.Edu).To(BeFalse())
			})

			It("should create a billing profile for the organisation", func() {
				res := doRequest()
				Expect(res.Code).To(Equal(http.StatusCreated))
				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())
				bp, err := billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())
				Expect(bp.State).To(Equal(billingprofile.StateNoCard))
			})

			It("responds with a json representation of the org that's been created", func() {
				res := doRequest()

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
				Expect(j.Org.BillingProfile).ToNot(BeNil())
				Expect(j.Org.BillingProfile.State).To(Equal(billingprofile.StateNoCard))

				Expect(j.Org.Role).To(Equal(orgmembership.RoleOwner))
			})

			It("adds the user as the owner of the org", func() {
				doRequest()

				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())

				Expect(m).To(HaveLen(1))
				Expect(m[0].Username.String).To(Equal("foo"))
				Expect(m[0].Role).To(Equal("owner"))
			})

			It("should make the org an edu if email is in the whitelist", func() {
				_, err := db.Exec("INSERT INTO edu_emails_whitelist(email) VALUES ('accounting@nitrous.io');")
				Expect(err).To(BeNil())

				doRequest()

				o, err := org.FindBySlug(db, "nitrous")
				Expect(err).To(BeNil())
				Expect(o.Edu).To(BeTrue())

				m, err := orgmembership.FindAllByOrgID(db, o.ID)
				Expect(err).To(BeNil())

				Expect(m).To(HaveLen(1))
				Expect(m[0].Username.String).To(Equal("foo"))
				Expect(m[0].Role).To(Equal("owner"))
			})
		})

		It("should return an error if the intent was edu but email was not whitelisted", func() {
			_, err := db.Exec("INSERT INTO edu_emails_whitelist(email) VALUES ('accounting@nitrous.gov');")
			Expect(err).To(BeNil())

			params.Set("intent", "edu")

			res := doRequest()
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("not_edu_whitelisted"))
			Expect(errDesc).To(Equal("Please contact Nitrous to enable education support"))

			_, err = org.FindBySlug(db, "nitrous")
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("responds with HTTP 400 Bad Request when slug is taken by another org", func() {
			res := doRequest() // Create org.
			Expect(res.Code).To(Equal(http.StatusCreated))

			_, err := org.FindBySlug(db, "nitrous")
			Expect(err).To(BeNil())

			res = doRequest() // Try to create org with the same slug.
			Expect(res.Code).To(Equal(http.StatusBadRequest))

			var j map[string]string
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("validation_failed"))
			Expect(j["error_description"]).To(Equal("slug is taken"))
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
						})

						It("responds with HTTP 400 Bad Request", func() {
							res := doRequest()
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
			})

			It("responds with 400 bad request", func() {
				res := doRequest()
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

	Describe("PutMember", func() {
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

		var doRequest func(string, string)

		itWorks := func() {
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
								_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), t.myRole)
								Expect(err).To(BeNil())
								_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), t.from)
								Expect(err).To(BeNil())

								params.Set("role", t.to)
							})

							itAllowsOrDisallowsUpdate := func() {
								if t.allowed {
									It("allows update", func() {
										Expect(res.Code).To(Equal(http.StatusOK))

										m, err := orgmembership.FindAllByOrgID(db, o.ID)
										Expect(err).To(BeNil())
										Expect(m).To(HaveLen(2))
										Expect(m[0].Username.String).To(Equal(u.Username))
										Expect(m[0].Role).To(Equal(t.myRole))
										Expect(m[1].Username.String).To(Equal(u2.Username))
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
										Expect(m[0].Username.String).To(Equal(u.Username))
										Expect(m[0].Role).To(Equal(t.myRole))
										Expect(m[1].Username.String).To(Equal(u2.Username))
										// role shouldn't have been updated
										Expect(m[1].Role).To(Equal(t.from))

										m, err = orgmembership.FindAllByOrgID(db, o2.ID)
										Expect(err).To(BeNil())
										Expect(m).To(HaveLen(0))
									})
								}
							}

							Context("with username", func() {
								BeforeEach(func() {
									doRequest(o.Slug, u2.Username)
								})

								itAllowsOrDisallowsUpdate()
							})

							Context("with email", func() {
								BeforeEach(func() {
									doRequest(o.Slug, u2.Email)
								})

								itAllowsOrDisallowsUpdate()
							})
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
								_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), t.myRole)
								Expect(err).To(BeNil())

								params.Set("role", t.role)
							})

							itAllowsOrDisallowsAddition := func() {
								if t.allowed {
									It("allows addition", func() {
										Expect(res.Code).To(Equal(http.StatusCreated))

										m, err := orgmembership.FindAllByOrgID(db, o.ID)
										Expect(err).To(BeNil())
										Expect(m).To(HaveLen(2))
										Expect(m[0].Username.String).To(Equal(u.Username))
										Expect(m[0].Role).To(Equal(t.myRole))
										// new org member should have been added with correct role
										Expect(m[1].Username.String).To(Equal(u2.Username))
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
										Expect(m[0].Username.String).To(Equal(u.Username))
										Expect(m[0].Role).To(Equal(t.myRole))

										m, err = orgmembership.FindAllByOrgID(db, o2.ID)
										Expect(err).To(BeNil())
										Expect(m).To(HaveLen(0))
									})
								}
							}

							Context("with username", func() {
								BeforeEach(func() {
									doRequest(o.Slug, u2.Username)
								})

								itAllowsOrDisallowsAddition()
							})

							Context("with email", func() {
								BeforeEach(func() {
									doRequest(o.Slug, u2.Email)
								})

								itAllowsOrDisallowsAddition()
							})
						})
					}(t)
				}
			})
		}

		Describe("PUT /api/v0/orgs/{slug}/members", func() {
			doRequest = func(slug, username string) {
				params.Set("username", username)
				var err error
				url := fmt.Sprintf("/api/v0/orgs/%s/members", slug)
				req, err = test.NewFormRequest("PUT", url, params)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			itWorks()

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = "/api/v0/orgs/foobar/members"
					authTokenInputs.Method = "PUT"
				})

				test.ItRequiresAuthToken(&authTokenInputs)
			})
		})

		Describe("PUT /api/v0/orgs/{slug}/members/{identifier}", func() {
			doRequest = func(slug, identifier string) {
				var err error
				url := fmt.Sprintf("/api/v0/orgs/%s/members/%s", slug, identifier)
				req, err = test.NewFormRequest("PUT", url, params)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			itWorks()

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = "/api/v0/orgs/foobar/members/foo"
					authTokenInputs.Method = "PUT"
				})

				test.ItRequiresAuthToken(&authTokenInputs)
			})

			Context("when org member added by email", func() {
				var (
					fakeMailer                 *test.FakeMailer
					origMailer, origOrgsMailer mailer.Mailer
				)

				BeforeEach(func() {
					envvars.WebHostname = "nitrous.example"
					origMailer = user.Mailer
					origOrgsMailer = orgs.Mailer
					fakeMailer = &test.FakeMailer{}
					orgs.Mailer = fakeMailer
				})

				AfterEach(func() {
					user.Mailer = origMailer
					orgs.Mailer = origOrgsMailer
				})

				It("should not invite user to Nitrous if blacklisted", func() {
					// Relies on `nitrous/.env-example` and `nitrous/dev/email-blacklist.txt`
					doRequest(o.Slug, "robin@yopmail.fr")

					Expect(res.Code).To(Equal(http.StatusNotFound))

					errSlug, errDesc := test.ErrorSlug(res.Body)
					Expect(errSlug).To(Equal("user_not_found"))
					Expect(errDesc).To(Equal("the specified user could not be found"))

					m, err := orgmembership.FindAllByOrgID(db, o.ID)
					Expect(err).To(BeNil())
					Expect(m).To(HaveLen(0))

					Expect(fakeMailer.SendMailCalled).To(BeFalse())
				})

				Context("when the given email is already a pending member", func() {
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

					runTestCases := func(t tc) {
						Context(fmt.Sprintf("when %s requests update of a member's role from %s to %s", t.myRole, t.from, t.to), func() {
							BeforeEach(func() {
								_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), t.myRole)
								Expect(err).To(BeNil())
								_, err = orgmembership.AddMember(db, o.ID, "bob@pending.com", t.from)
								Expect(err).To(BeNil())

								params.Set("role", t.to)

								doRequest(o.Slug, "bob@pending.com")
							})

							if t.allowed {
								It("allows update", func() {
									Expect(res.Code).To(Equal(http.StatusOK))

									m, err := orgmembership.FindAllByOrgID(db, o.ID)
									Expect(err).To(BeNil())
									Expect(m).To(HaveLen(2))
									Expect(m[0].Username.String).To(Equal(u.Username))
									Expect(m[0].Role).To(Equal(t.myRole))
									Expect(m[1].Username.Valid).To(BeFalse())
									Expect(m[1].InviteEmail.String).To(Equal("bob@pending.com"))
									Expect(m[1].InviteCode.Valid).To(BeTrue())
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
									Expect(j.Members[1].UserID).To(BeZero())
									Expect(j.Members[1].PendingEmail).To(Equal("bob@pending.com"))
									Expect(j.Members[1].Role).To(Equal(t.to))

									Expect(fakeMailer.SendMailCalled).To(BeFalse())
								})
							} else {
								It("disallows update", func() {
									Expect(res.Code).To(Equal(http.StatusForbidden))

									m, err := orgmembership.FindAllByOrgID(db, o.ID)
									Expect(err).To(BeNil())
									Expect(m).To(HaveLen(2))
									Expect(m[0].Username.String).To(Equal(u.Username))
									Expect(m[0].Role).To(Equal(t.myRole))
									Expect(m[1].Username.Valid).To(BeFalse())
									// role shouldn't have been updated
									Expect(m[1].Role).To(Equal(t.from))

									m, err = orgmembership.FindAllByOrgID(db, o2.ID)
									Expect(err).To(BeNil())
									Expect(m).To(HaveLen(0))

									Expect(fakeMailer.SendMailCalled).To(BeFalse())
								})
							}
						})
					}

					for _, t := range tcs {
						runTestCases(t)
					}
				})

				It("should send invite email", func() {
					_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
					Expect(err).To(BeNil())

					doRequest(o.Slug, "bob@abc.com")

					Expect(res.Code).To(Equal(http.StatusCreated))

					Eventually(func() interface{} {
						orgMember := &orgmembership.OrgMembership{}
						err := db.Get(orgMember, "SELECT * FROM org_memberships WHERE org_id = $1 AND invite_email = $2", o.ID, "bob@abc.com")
						Expect(err).To(BeNil())
						Expect(orgMember).ToNot(BeNil())
						Expect(orgMember.UserID.Valid).To(BeFalse())

						return orgMember.EmailSentAt.Valid
					}).Should(BeTrue())

					Expect(fakeMailer.SendMailCalled).To(BeTrue())
					Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
					Expect(fakeMailer.Tos).To(Equal([]string{"bob@abc.com"}))
					Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
					Expect(fakeMailer.Subject).To(Equal(fmt.Sprintf("%s has invited you to organization %s!", u.Name, o.Name)))

					Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("%s invited you to join the %s organization on Nitrous.", u.Name, o.Name)))
					Expect(fakeMailer.HTML).To(ContainSubstring("You will be able to create projects"))
					Expect(fakeMailer.HTML).To(ContainSubstring("/signup?invite="))

					Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("%s invited you to join the %s organization on Nitrous.", u.Name, o.Name)))
					Expect(fakeMailer.Body).To(ContainSubstring("You will be able to create projects"))
					Expect(fakeMailer.Body).To(ContainSubstring("/signup?invite="))
				})
			})
		})
	})

	Describe("DELETE /api/v0/orgs/{slug}/members/{identifier}", func() {
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
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "admin")
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "member")
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
			It("allows a regular member to be deleted", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "member")
				Expect(err).To(BeNil())

				doRequest(o.Slug, u.Username)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o.Slug)
				Expect(err).To(Equal(sql.ErrNoRows))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["removed"]).To(BeTrue())
			})

			It("allows an admin to be deleted", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "admin")
				Expect(err).To(BeNil())

				doRequest(o.Slug, u.Username)
				Expect(res.Code).To(Equal(http.StatusOK))

				_, err = orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o.Slug)
				Expect(err).To(Equal(sql.ErrNoRows))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["removed"]).To(BeTrue())
			})

			It("should not allow an owner to be deleted", func() {
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
				Expect(err).To(BeNil())

				doRequest(o.Slug, u.Username)
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				_, err = orgmembership.FindByIdentifierAndOrgSlug(db, u.Username, o.Slug)
				Expect(err).To(BeNil())

				j := map[string]string{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_action"))
				Expect(j["error_description"]).To(Equal("you cannot remove yourself from this organization"))
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
							_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), t.myRole)
							Expect(err).To(BeNil())
							_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u2.ID, 10), t.targetUserRole)
							Expect(err).To(BeNil())

							doRequest(o.Slug, u2.Username)
						})

						if t.allowed {
							It("allows deletion", func() {
								Expect(res.Code).To(Equal(http.StatusOK))

								m, err := orgmembership.FindAllByOrgID(db, o.ID)
								Expect(err).To(BeNil())
								Expect(m).To(HaveLen(1))
								Expect(m[0].Username.String).To(Equal(u.Username))
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
								Expect(m[0].Username.String).To(Equal(u.Username))
								Expect(m[0].Role).To(Equal(t.myRole))
								Expect(m[1].Username.String).To(Equal(u2.Username))
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
			err = db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o2.ID, true})).To(BeNil())
				return nil
			})
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
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "member")
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
				Expect(j.Org.BillingProfile).To(BeNil())
			})

			It("includes plan information if the org is subscribed to a multi host plan", func() {
				pl := &plan.Plan{Name: "Large", Slug: "large", Price: 500}
				Expect(pl.Insert(db)).To(BeNil())

				oBP, err := billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())

				oS := &subscription.Subscription{
					PlanID:           pl.ID,
					BillingProfileID: oBP.ID,
					State:            "active",
					StripeID:         sql.NullString{"orgSub", true},
					Slug:             "SUB1",
					MultiHost:        true,
				}
				Expect(test.WrapInTransaction(db, oS.Insert)).To(BeNil())

				doRequest(o.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Org *serializers.Org `json:"org"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Org.MultiHost).To(BeTrue())
				Expect(j.Org.Plan).ToNot(BeNil())
				Expect(j.Org.Plan.ID).To(Equal(pl.ID))
			})

			It("includes the billing profile if the user is an admin", func() {
				for _, role := range []string{"owner", "admin"} {
					_, err := db.Exec("UPDATE org_memberships SET role = $1 WHERE org_id = $2 and user_id = $3", role, o.ID, u.ID)
					Expect(err).To(BeNil())

					doRequest(o.Slug)
					Expect(res.Code).To(Equal(http.StatusOK))

					var j struct {
						Org *serializers.Org `json:"org"`
					}
					err = json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())

					Expect(j.Org.BillingProfile).ToNot(BeNil())
					Expect(j.Org.BillingProfile.State).To(Equal(billingprofile.StateNoCard))
				}
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

			_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "owner")
			Expect(err).To(BeNil())
			_, err = orgmembership.AddMember(db, o2.ID, strconv.FormatInt(u2.ID, 10), "member")
			Expect(err).To(BeNil())
			_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u3.ID, 10), "admin")
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
			It("responds with 404 not found if the user is limited", func() {
				_, err := db.Exec("UPDATE org_memberships SET role=$1 WHERE user_id = $2", orgmembership.RoleLimited, u.ID)
				Expect(err).To(BeNil())

				doRequest(o.Slug)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, _ := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
			})

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
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), "member")
				Expect(err).To(BeNil())

				_, err = orgmembership.AddMember(db, o3.ID, strconv.FormatInt(u.ID, 10), "member")
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

			It("includes plan information if the org is subscribed to a multi host plan", func() {
				pl := &plan.Plan{Name: "Large", Slug: "large", Price: 500}
				Expect(pl.Insert(db)).To(BeNil())

				err := db.WithTransaction(func(tx *sqlx.Tx) error {
					return billingprofile.Create(tx, u.ID, sql.NullInt64{o3.ID, true})
				})
				Expect(err).To(BeNil())

				oBP, err := billingprofile.FindForOrg(db, o3.ID)
				Expect(err).To(BeNil())

				oS := &subscription.Subscription{
					PlanID:           pl.ID,
					BillingProfileID: oBP.ID,
					State:            "active",
					StripeID:         sql.NullString{"orgSub", true},
					Slug:             "SUB1",
					MultiHost:        true,
				}
				Expect(test.WrapInTransaction(db, oS.Insert)).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Orgs []*serializers.Org `json:"orgs"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Orgs).To(HaveLen(2))

				Expect(j.Orgs[1].ID).To(Equal(o3.ID))
				Expect(j.Orgs[1].MultiHost).To(BeTrue())
				Expect(j.Orgs[1].Plan).ToNot(BeNil())
				Expect(j.Orgs[1].Plan.ID).To(Equal(pl.ID))
			})

			It("returns the billing profile if a member can see the billing profile", func() {
				err := db.WithTransaction(func(tx *sqlx.Tx) error {
					return billingprofile.Create(tx, u.ID, sql.NullInt64{o.ID, true})
				})
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE billing_profiles SET last_four = '2334' WHERE org_id = $1", o.ID)
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE org_memberships SET role = $1 WHERE user_id = $2 AND org_id = $3", orgmembership.RoleAdmin, u.ID, o.ID)
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Orgs []*serializers.Org `json:"orgs"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.Orgs).To(HaveLen(2))
				Expect(j.Orgs[0].ID).To(Equal(o.ID))
				Expect(j.Orgs[0].Email).To(Equal(o.Email))
				Expect(j.Orgs[0].Name).To(Equal(o.Name))
				Expect(j.Orgs[0].Slug).To(Equal(o.Slug))
				Expect(j.Orgs[0].BillingProfile).ToNot(BeNil())
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
