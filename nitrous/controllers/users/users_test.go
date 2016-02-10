package users_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gopkg.in/redis.v2"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/controllers/users"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/verification"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/logintoken"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	redistest "github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/users")
}

var _ = Describe("controllers/users", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		params          *url.Values
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
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}
		u = &user.User{
			Username: "tokenuser",
			Password: "123456",
			Email:    "tokenuser@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "foobarbazqux",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		users.CVerifier = &test.FakeCaptchaVerifier{}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/users/email_signup", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/users/email_signup", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should respond OK if no email is provided", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))
		})

		It("should respond OK if the email is invalid", func() {
			params = &url.Values{}
			params.Set("email", "foo.com")
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))
		})

		It("should create an email record if the email is fine", func() {
			params = &url.Values{}
			params.Set("email", "bar@foo.com")
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]string{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM email_captures WHERE email_address = $1;`, "bar@foo.com").Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})
	})

	Describe("POST /api/v0/users", func() {
		doRequest := func() {
			var err error
			req, err = test.NewFormRequest("POST", "/api/v0/users", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("email", "foo@example.com")
			params.Set("username", "foo")
			params.Set("password", "123456")
			params.Set("name", "Foo Bar")
			params.Set("promo_code", "")
			params.Set("captcha_token", "catpcha-token")
		})

		Context("when valid params are received", func() {
			BeforeEach(func() {
				doRequest()
			})

			It("creates a user record in the db", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Name).To(Equal("Foo Bar"))
				Expect(u.CreatedAt).NotTo(BeNil())
				Expect(u.Suspicious.Valid).To(BeFalse())

				Expect(ft.ProfileCreated).ToNot(BeNil())
				Expect(ft.ProfileCreated.ID).To(Equal(u.ID))
			})

			It("encrypts the password using bcrypt", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(bcrypt.Verify(u.EncryptedPassword, "123456")).To(BeTrue())
			})

			It("responds with created and serialized user", func() {
				var j struct {
					User *serializers.User `json:"user"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.User.Username).To(Equal("foo"))
				Expect(j.User.Email).To(Equal("foo@example.com"))
			})
		})

		Context("when remote address is in CIDR", func() {
			It("marks the user as suspicious if remote address is in CIDR", func() {
				params = &url.Values{}
				params.Set("email", "foo@example.com")
				params.Set("username", "foo")
				params.Set("password", "123456")
				params.Set("name", "Foo Bar")
				params.Set("promo_code", "")

				var err error
				req, err = test.NewFormRequest("POST", "/api/v0/users", params)
				req.RemoteAddr = "1.2.3.4" // trigger CIDR block
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)
				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.Suspicious.String).To(Equal("blocked by CIDR 1.2.3.0/24"))
			})
		})

		for _, v := range []string{"email", "username", "password"} {
			Context(fmt.Sprintf(`when the required param "%s" is missing`, v), func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("email", "foo@example.com")
					params.Set("username", "foo")
					params.Set("password", "123456")
					params.Del(v)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal(v + " can't be blank"))
				})
			})
		}

		Context("when other types of validation fails", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foobar")
				params.Set("username", "foo")
				params.Set("password", "123456")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("email is not valid"))
			})
		})

		Context("using promocodes", func() {
			var (
				pcID int64
			)

			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foo@example.com")
				params.Set("username", "foo")
				params.Set("password", "123456")
				params.Set("name", "Foo Bar")
				params.Set("promo_code", "magic")

				err := db.Get(&pcID, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('magic', 10, 900, now()+interval '2 days') RETURNING id;")
				Expect(err).To(BeNil())
			})

			It("should return an error if the promocode is invalid", func() {
				_, err := db.Exec("UPDATE promo_codes SET expires_at = now() - interval '1 hour';")
				Expect(err).To(BeNil())

				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errSlug, errDesc := test.ErrorSlug(res.Body)
				Expect(errSlug).To(Equal("invalid_promo_code"))
				Expect(errDesc).To(Equal("promo code does not exist"))
			})

			It("should assign the promocode against the user if valid", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())

				var count int
				err = db.Get(&count, `SELECT COUNT(*) FROM promo_code_uses
					WHERE promo_code_id = $1 AND user_id = $2 AND subscription_id IS NULL;`, pcID, u.ID)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("with an existing user", func() {
			BeforeEach(func() {
				_, err := db.Exec("INSERT INTO users (username, email, encrypted_password) VALUES ('foo', 'foo@example.com', '!@#$%')")
				Expect(err).To(BeNil())
				params = &url.Values{}
				params.Set("email", "lol@example.com")
				params.Set("username", "bar")
				params.Set("password", "123456")
			})

			Context("when an existing username is given", func() {
				BeforeEach(func() {
					params.Set("username", "foo")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("username is taken"))
				})
			})

			Context("when an existing email is given", func() {
				BeforeEach(func() {
					params.Set("email", "foo@example.com")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("email is taken"))
				})
			})
		})

		It("should not allow banned email addresses", func() {
			b := &user.User{
				Username: "banner",
				Password: "123456",
				Email:    "banner@example.com",
			}
			err := test.WrapInTransaction(db, b.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO users (username, email, encrypted_password, deleted_at, banned_at, banned_by_id) VALUES ('foo', 'foo@example.com', '!@#$%', now(), now(), $1)", b.ID)
			Expect(err).To(BeNil())

			params = &url.Values{}
			params.Set("email", "foo@example.com")
			params.Set("username", "foo")
			params.Set("password", "123456")
			params.Set("name", "Foo Bar")
			doRequest()

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]string{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("email_banned"))
			Expect(j["error_description"]).To(Equal("this email address is banned"))
		})

		Context("invited by container collaborator", func() {
			var (
				cnr *container.Container
				cc  *containercollab.ContainerCollab
			)

			BeforeEach(func() {
				owner := &user.User{
					Username: "owner",
					Email:    "owner@example.com",
					Password: "123456",
				}
				err := test.WrapInTransaction(db, owner.Insert)
				Expect(err).To(BeNil())

				hs, err := test.CreateHostStack(db, &test.HostStack{
					U: owner,
				})
				Expect(err).To(BeNil())

				cnr = &container.Container{
					CID:    "cafebabe",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
					Image:  "nitrous-base",
					Slug:   "java-test-container",
					Name:   sql.NullString{"Java", true},
				}
				err = cnr.Insert(db)
				Expect(err).To(BeNil())

				cc, err = containercollab.Create(db, cnr.ID, "foo@example.com")
				Expect(err).To(BeNil())
			})

			It("should register the user as a collab member by updating container_collabs if invite_code provided with correct email", func() {
				params.Set("invite_code", cc.InviteCode.String)
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeTrue())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeTrue())
			})

			It("should not register the user as a collab member if the invite code is incorrect", func() {
				params.Set("invite_code", "23x2")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeFalse())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeFalse())
			})

			It("should not register the user as a collab member if the email is different", func() {
				params.Set("email", "foo@domain.com")
				params.Set("invite_code", "22x2")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeFalse())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM container_collabs WHERE user_id = $1 AND container_id = $2`, u.ID, cnr.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeFalse())
			})
		})

		Context("invited by org owner", func() {
			var (
				o *org.Org
			)

			BeforeEach(func() {
				o = &org.Org{
					Name:  "Foo Bar, Inc.",
					Slug:  "foobar",
					Email: "eng@foobar.com",
				}
				err := o.Insert(db)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO org_memberships (org_id, invite_email, invite_code) VALUES ($1, 'foo@example.com', '22x2') RETURNING ID`, o.ID)
				Expect(err).To(BeNil())
			})

			It("should register the user by updating org_memberships if invite_code provided with correct email", func() {
				params.Set("invite_code", "22x2")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeTrue())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM org_memberships WHERE user_id = $1 AND org_id = $2`, u.ID, o.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeTrue())
			})

			It("should not register the user under the org if the invite code is incorrect", func() {
				params.Set("invite_code", "23x2")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeFalse())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM org_memberships WHERE user_id = $1 AND org_id = $2`, u.ID, o.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeFalse())
			})

			It("should not register the user under the org if the email is different", func() {
				params.Set("email", "foo@domain.com")
				params.Set("invite_code", "22x2")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u.ConfirmedAt.Valid).To(BeFalse())

				var found bool
				err = db.Get(&found, `SELECT COUNT(*) = 1 FROM org_memberships WHERE user_id = $1 AND org_id = $2`, u.ID, o.ID)
				Expect(err).To(BeNil())
				Expect(found).To(BeFalse())
			})
		})

		Context("creation by oauth client", func() {
			var (
				client       *oauthclient.Client
				clientSecret = "BUILDxcvv45vs2"
			)

			BeforeEach(func() {
				client = &oauthclient.Client{
					UserID:      u.ID,
					Name:        "Builders League United",
					Description: "A fictional company",
					ClientID:    "blu_team",
					RedirectURI: "https://blu.co/oauth/callback",
				}
				Expect(client.InsertWithSecret(db, clientSecret)).To(BeNil())

				params.Set("client_id", client.ClientID)
				params.Set("client_secret", clientSecret)
				params.Set("remote_addr", "142.254.116.10")
				params.Set("captcha_token", "")

				_, err := db.Exec("UPDATE oauth_clients SET create_user = TRUE WHERE id = $1", client.ID)
				Expect(err).To(BeNil())
			})

			It("should return not found if the client ID is incorrect", func() {
				params.Set("client_id", "not_even")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("should return not found if the client secret is incorrect", func() {
				params.Set("client_secret", "wrong")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("should return not found if the client does not have permission to create users", func() {
				_, err := db.Exec("UPDATE oauth_clients SET create_user = FALSE WHERE id = $1", client.ID)
				Expect(err).To(BeNil())
				doRequest()

				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("should require the remote_addr be provided", func() {
				params.Set("remote_addr", "")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_remote_addr"))
				Expect(errDesc).To(Equal("remote_addr must be provided"))
			})

			It("should create the user as suspicious if the provided remote_addr is blacklisted", func() {
				params.Set("remote_addr", "1.2.3.4")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.Suspicious.String).To(Equal("blocked by CIDR 1.2.3.0/24"))
			})

			It("should create the user", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusCreated))

				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.Suspicious.Valid).To(BeFalse())
				Expect(u.CreatedByClientID.Int64).To(Equal(client.ID))
				Expect(u.ConfirmedAt.Valid).To(BeTrue())
			})

			Context("with an org", func() {
				var o *org.Org

				BeforeEach(func() {
					o = &org.Org{
						Name:  "Foo Bar, Inc.",
						Slug:  "foobar",
						Email: "eng@foobar.com",
					}
					err := o.Insert(db)
					Expect(err).To(BeNil())

					_, err = db.Exec("UPDATE oauth_clients SET allowed_org_slugs = 'foobar' WHERE id = $1", client.ID)
					Expect(err).To(BeNil())

					params.Set("org_slug", "foobar")
					params.Set("org_permission", orgmembership.RoleLimited)
				})

				It("should not allow an Admin membership", func() {
					params.Set("org_permission", orgmembership.RoleAdmin)

					doRequest()

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_org_permission"))
					Expect(errDesc).To(Equal("org_permission must be `member`"))
				})

				It("should not allow an Owner membership", func() {
					params.Set("org_permission", orgmembership.RoleOwner)

					doRequest()

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_org_permission"))
					Expect(errDesc).To(Equal("org_permission must be `member`"))
				})

				It("should require the org slug be allowed for the client", func() {
					params.Set("org_slug", "fooebar")

					doRequest()

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("invalid_org_slug"))
					Expect(errDesc).To(Equal("org_slug is not allowed"))
				})

				It("should require the org exist", func() {
					_, err := db.Exec("UPDATE oauth_clients SET allowed_org_slugs = 'fooebar' WHERE id = $1", client.ID)
					Expect(err).To(BeNil())
					params.Set("org_slug", "fooebar")

					doRequest()

					Expect(res.Code).To(Equal(http.StatusNotFound))
					errType, errDesc := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("org_not_found"))
					Expect(errDesc).To(Equal("org not found"))
				})

				It("should add the user as a limited member", func() {
					doRequest()

					Expect(res.Code).To(Equal(http.StatusCreated))

					mem, err := orgmembership.FindByIdentifierAndOrgSlug(db, "foo", "foobar")
					Expect(err).To(BeNil())
					Expect(mem.Role).To(Equal(orgmembership.RoleLimited))
				})

				It("should allow for a user to be added as a member", func() {
					params.Set("org_permission", orgmembership.RoleMember)
					doRequest()

					Expect(res.Code).To(Equal(http.StatusCreated))

					mem, err := orgmembership.FindByIdentifierAndOrgSlug(db, "foo", "foobar")
					Expect(err).To(BeNil())
					Expect(mem.Role).To(Equal(orgmembership.RoleMember))
				})
			})
		})

		Context("github username is specified", func() {
			var (
				redisClient *redis.Client
				err         error
			)

			BeforeEach(func() {
				params.Set("github_username", "github-user")
				redisClient, err = redistest.PrepareRedis(envvars.RedisHost, envvars.RedisPort, envvars.RedisDB, true)
				Expect(err).NotTo(HaveOccurred())

				gitHubUserInfo := github.GitHubUserInfo{
					UserName:  "github-user",
					TokenJSON: `{ "access_token": "keytogithub" }`,
				}

				b, err := json.Marshal(gitHubUserInfo)
				Expect(err).NotTo(HaveOccurred())

				_, err = redisClient.Set("github:userinfo:github-user", string(b)).Result()
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if redisClient != nil {
					redisClient.Close()
				}
			})

			It("updates user profile with github info", func() {
				doRequest()
				up, err := userprofile.FindByGitHubUserName(db, "github-user")
				Expect(err).NotTo(HaveOccurred())

				Expect(up.GitHubUserName.String).To(Equal("github-user"))
				Expect(up.GitHubOAuthTokenJson.String).To(Equal(`{ "access_token": "keytogithub" }`))
				Expect(up.GitHubUserScopesJson.String).To(Equal(`["user:email","repo","admin:public_key"]`))
			})

			It("removes github info from redis", func() {
				doRequest()
				val, err := redisClient.Exists("github:userinfo:github-user").Result()
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeFalse())
			})

			Context("the github is not preset in redis", func() {
				BeforeEach(func() {
					_, err = redisClient.Del("github:userinfo:github-user").Result()
					Expect(err).NotTo(HaveOccurred())
				})

				It("does not create a user", func() {
					doRequest()
					_, err := user.FindByUsername(db, "foo")
					Expect(err).To(Equal(sql.ErrNoRows))
				})
			})
		})

		Context("captcha token is invalid", func() {
			BeforeEach(func() {
				users.CVerifier = &test.FakeCaptchaVerifier{Err: errors.New("failed to verify")}
			})

			It("responds with 400 bad request", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_captcha_token"))
				Expect(j["error_description"]).To(Equal("failed to verify captcha token"))
			})
		})
	})

	Describe("PUT /api/v0/user", func() {
		doRequest := func(params *url.Values) *httptest.ResponseRecorder {
			var err error
			req, err = test.NewAuthedFormRequest(t, "PUT", "/api/v0/users", params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			return res
		}

		It("should block if the user has billing issues", func() {
			test.MarkDelinquentUser(db, u)
			params := &url.Values{}
			params.Set("name", "Something")
			res := doRequest(params)
			test.VerifyDelinquentUserResponse(res)
		})

		Context("when valid fields are given", func() {
			var params *url.Values

			BeforeEach(func() {
				params = &url.Values{}
				params.Set("password", "abcdef")
				params.Set("email", "tupdate@example.com")
				params.Set("name", "Token Update")
			})

			It("updates fields", func() {
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))
				updated, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(bcrypt.Verify(updated.EncryptedPassword, "abcdef")).To(BeTrue())
				Expect(updated.Name).To(Equal("Token Update"))
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *u.UpdatedAt))
			})

			It("should not allow update if token scope is not full", func() {
				_, err := db.Exec(`UPDATE tokens SET scope = 'third_party' WHERE id = $1;`, t.ID)
				Expect(err).To(BeNil())

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusForbidden))
			})

			It("responds with the user, user profile, and billing profile", func() {
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					User serializers.User `json:"user"`
				}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				serialized := j.User
				Expect(serialized.ID).To(Equal(u.ID))
				Expect(serialized.Username).To(Equal(u.Username))
				Expect(serialized.UserProfile).NotTo(BeNil())
				Expect(serialized.UserProfile.UserID).To(Equal(u.ID))
				Expect(serialized.BillingProfile).NotTo(BeNil())
				Expect(serialized.BillingProfile.UserID).To(Equal(u.ID))
			})
		})

		It("should remove all other login tokens on password change", func() {
			_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u.ID, "toke1")
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("password", "abcdef")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			type con struct {
				Token string `db:"token"`
			}
			tokens := []con{}
			err = db.Select(&tokens, `SELECT token FROM tokens WHERE user_id = $1`, u.ID)
			Expect(err).To(BeNil())

			Expect(tokens).To(ConsistOf([]con{{Token: t.Token}}))
		})

		It("should return an error if existing_password is provided and incorrect", func() {
			_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u.ID, "toke1")
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("password", "abcdef")
			params.Set("existing_password", "iamwrong")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})

		It("should not return an error if existing_password is provided and is correct", func() {
			_, err := db.Exec("INSERT INTO tokens (user_id, token) values ($1, $2);", u.ID, "toke1")
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("password", "abcdef")
			params.Set("existing_password", "123456")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		Context("when an existing email is given", func() {
			BeforeEach(func() {
				u = &user.User{
					Username: "taken",
					Password: "123456",
					Email:    "taken@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())
			})

			It("responds with 400 bad request", func() {
				params := &url.Values{}
				params.Set("email", "taken@example.com")
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("validation_failed"))
				Expect(errDesc).To(Equal("email is taken"))
			})
		})

		Context("when validation fails", func() {
			It("responds with 400 bad request", func() {
				params := &url.Values{}
				params.Set("password", "a")

				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := test.ErrorSlug(res.Body)
				Expect(errType).To(Equal("validation_failed"))
				Expect(errDesc).To(ContainSubstring("too short"))
			})
		})

		Context("when username is given", func() {
			It("does not update", func() {
				params := &url.Values{}
				params.Set("username", "nocando")

				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))
				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.Username).To(Equal("tokenuser"))
				Expect(usr.UpdatedAt).To(Equal(u.UpdatedAt))
			})
		})

		Context("when an email is sent in the request", func() {
			var (
				fakeMailer *test.FakeMailer
				origMailer mailer.Mailer
			)

			BeforeEach(func() {
				_, err := db.Exec(`UPDATE users SET confirmed_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				origMailer = user.Mailer
				fakeMailer = &test.FakeMailer{}
				user.Mailer = fakeMailer
			})

			It("should not fire off an email if the email is the same", func() {
				params := &url.Values{}
				params.Set("email", u.Email)
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(BeEmpty())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should fire off an email if the email differs from the current value", func() {
				params := &url.Values{}
				params.Set("email", "att@example.com")
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(Equal("att@example.com"))

				Expect(fakeMailer.SendMailCalled).To(BeTrue())
				Expect(fakeMailer.Tos).To(Equal([]string{`"tokenuser" <att@example.com>`}))
			})

			It("should not fire off an email if the email is the same as the unconfirmed email", func() {
				_, err := db.Exec(`UPDATE users SET unconfirmed_email = 'att@example.com' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("email", "att@example.com")
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(Equal("att@example.com"))

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})

			It("should revert to the original email if provided while unconfirmed is different", func() {
				_, err := db.Exec(`UPDATE users SET confirmation_token = 'TOKEN', confirmation_sent_at = now(), confirmed_at = now(), unconfirmed_email = 'att@example.com' WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				params := &url.Values{}
				params.Set("email", u.Email)
				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				usr, err := user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(usr.UnconfirmedEmail.String).To(BeEmpty())
				Expect(usr.ConfirmedAt.Valid).To(BeTrue())
				Expect(usr.ConfirmationSentAt.Valid).To(BeFalse())
				Expect(usr.ConfirmationToken.String).To(BeEmpty())

				Expect(fakeMailer.SendMailCalled).To(BeFalse())
			})
		})
	})

	Describe("POST /api/v0/user/phone", func() {
		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/phone", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("updates the user's phone and phone_country_code", func() {
			params := &url.Values{}
			params.Set("phone", "(888)8-44-44 Ծ_Ծ") // test stripping of non-numeric
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Phone.String).To(Equal("88884444"))
			Expect(updated.PhoneCountryCode.String).To(Equal("65"))
			Expect(*updated.UpdatedAt).To(BeTemporally(">", *u.UpdatedAt))

			var j struct {
				User *serializers.User `json:"user"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			serialized := j.User
			Expect(serialized.Username).To(Equal("tokenuser"))
			Expect(serialized.Phone).To(Equal("88884444"))
			Expect(serialized.PhoneCountryCode).To(Equal("65"))
			Expect(serialized.PhoneAuthed).To(BeFalse())

			Expect(serialized.UserProfile).NotTo(BeNil())
			Expect(serialized.UserProfile.UserID).To(Equal(u.ID))
			Expect(serialized.BillingProfile).NotTo(BeNil())
			Expect(serialized.BillingProfile.UserID).To(Equal(u.ID))
		})

		It("should not allow update phone if token scope is not full", func() {
			_, err := db.Exec(`UPDATE tokens SET scope = 'third_party' WHERE id = $1;`, t.ID)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("phone", "(888)8-44-44")
			params.Set("phone_country_code", "65")
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))
		})

		It("creates a verification record for the user's phone", func() {
			// Confirm that user doesn't already have a verification record.
			v, err := verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(Equal(sql.ErrNoRows))

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			v, err = verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())
			Expect(v.UserID).To(Equal(u.ID))
			Expect(v.Phone).To(Equal("+6588884444"))
			Expect(v.Code).NotTo(BeEmpty())
		})

		It("responds with HTTP 400 Bad Request when phone is not given", func() {
			params := &url.Values{}
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request when phone_country_code is not given", func() {
			params := &url.Values{}
			params.Set("phone", "88884444")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request when phone & country code are banned", func() {
			b := &user.User{
				Username: "banner",
				Password: "123456",
				Email:    "banner@example.com",
			}
			err := test.WrapInTransaction(db, b.Insert)
			Expect(err).To(BeNil())

			u2 := &user.User{
				Username: "anotheruser",
				Password: "123456",
				Email:    "anotheruser@example.com",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			phone := "88884444"
			phoneCountryCode := "65"
			_, err = db.Exec(`UPDATE users
				SET deleted_at = now(), banned_at = now(), phone = $1, phone_country_code = $2, phone_authed = TRUE, banned_by_id = $3 WHERE id = $4;`,
				phone, phoneCountryCode, b.ID, u2.ID)

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("phone_banned"))
			Expect(errDesc).To(Equal("this phone number is banned"))
		})

		It("responds with HTTP 400 Bad Request when phone number is already in the database", func() {
			u2 := &user.User{
				Username: "anotheruser",
				Password: "123456",
				Email:    "anotheruser@example.com",
			}
			err := test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())
			phone := "88884444"
			phoneCountryCode := "65"
			_, err = db.Exec(`UPDATE users SET phone = $1, phone_country_code = $2, phone_authed = $3 WHERE id = $4;`, phone, phoneCountryCode, true, u2.ID)

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
			Expect(errDesc).To(Equal("phone number already exists"))
		})

		It("responds with HTTP 400 Bad Request when phone is already authed", func() {
			_, err := db.Exec(`
				UPDATE users SET phone = '5551231234', phone_country_code = '1', phone_authed = true WHERE id = $1
			`, u.ID)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("phone", "88884444")
			params.Set("phone_country_code", "65")
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("phone_authed"))

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.PhoneAuthed).To(BeTrue())
			Expect(u2.Phone.String).To(Equal("5551231234"))
		})
	})

	Describe("POST /api/v0/user/bonus", func() {
		var (
			threeHours = int64(3 * 60 * 60) // in seconds
			fiveHours  = int64(5 * 60 * 60)
		)

		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/bonus", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("updates the user's usage quota", func() {
			_, err := test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					UserID: u.ID,
					Free:   true,
				},
			})
			Expect(err).To(BeNil())

			bonuses := []struct {
				slug   string
				amount int64
			}{
				{"follow-twitter", threeHours},
				{"tweet", fiveHours},
				{"like-facebook", threeHours},
				{"share-facebook", fiveHours},
			}

			for _, bonus := range bonuses {
				_, err := db.Exec("INSERT INTO bonuses (slug, amount) VALUES ($1, $2);", bonus.slug, bonus.amount)
				Expect(err).To(BeNil())
			}

			params := &url.Values{}
			params.Set("action", "follow-twitter")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				User          *serializers.User          `json:"user"`
				EarnedBonuses []*serializers.EarnedBonus `json:"earned_bonuses"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			serializedUser := j.User
			serializedEarnedBonuses := j.EarnedBonuses

			Expect(serializedUser.UsageQuota).To(Equal(u.UsageQuota.Int64 + threeHours))
			Expect(serializedUser.UsageRemaining).To(BeNumerically("<=", u.UsageQuota.Int64+threeHours))

			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			Expect(serializedEarnedBonuses).To(HaveLen(1))
			Expect(serializedEarnedBonuses[0].UserID).To(Equal(u.ID))
			Expect(serializedEarnedBonuses[0].BonusID).To(Equal(bn.ID))
		})

		It("should return not found if the bonus type is not valid", func() {
			_, err := test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					UserID: u.ID,
					Free:   true,
				},
			})
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("action", "tweeter")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusNotFound))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("bonus_not_found"))
			Expect(errDesc).To(Equal("the bonus type is invalid"))
		})

		It("should return forbidden if the user has already earned the bonus", func() {
			_, err := test.CreateHostStack(db, &test.HostStack{
				U: u,
				H: &host.Host{
					UserID: u.ID,
					Free:   true,
				},
			})
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO bonuses (slug, amount) VALUES ($1, $2);", "follow-twitter", int64(3*24*60*60))
			Expect(err).To(BeNil())

			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				UserID:  u.ID,
				BonusID: bn.ID,
			}

			err = ebn.Insert(db)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("action", "follow-twitter")

			doRequest(params)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("forbidden"))
			Expect(errDesc).To(Equal("you have already earned this bonus"))
		})
	})

	Describe("POST /api/v0/user/resend_phone_code", func() {
		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/resend_phone_code", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("creates a new verification for the user's phone", func() {
			// Update user's phone and add an existing verification.
			err := u.UpdatePhone(db, "88884444", "65")
			Expect(err).To(BeNil())
			v1, err := verification.Generate(db, u.ID, u.PhoneNumber())
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			v2, err := verification.Find(db, u.ID, "+6588884444")
			Expect(err).To(BeNil())
			Expect(v2.Phone).To(Equal("+6588884444"))
			Expect(v2.Code).NotTo(Equal(v1.Code))

			// Verify response is { "sent": true }.
			j := map[string]bool{}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["sent"]).To(BeTrue())
		})

		It("responds with HTTP 400 Bad Request if user has not given us his phone number", func() {
			Expect(u.Phone.String).To(BeEmpty())
			Expect(u.PhoneCountryCode.String).To(BeEmpty())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("POST /api/v0/user/verify_phone", func() {
		doRequest := func(params *url.Values) {
			var err error
			req, err = test.NewAuthedFormRequest(t, "POST", "/api/v0/user/verify_phone", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		var v *verification.Verification

		BeforeEach(func() {
			var err error
			// Update user's phone and add an existing verification.
			err = u.UpdatePhone(db, "88884444", "65")
			Expect(err).To(BeNil())
			v, err = verification.Generate(db, u.ID, u.PhoneNumber())
			Expect(err).To(BeNil())
		})

		It("sets phone_authed to true if verification code is correct", func() {
			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			// Verify response is { "verified": true }.
			j := map[string]bool{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["verified"]).To(BeTrue())

			u2, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u2.PhoneAuthed).To(BeTrue())
		})

		It("sets user's billing profile to free tier if phone number belongs to a whitelisted country", func() {
			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateFree))
		})

		It("does not change user's billing profile to free tier if phone number does not belong to a whitelisted country", func() {
			// Put only Japan on the whitelist.
			orig := users.PhoneCountryWhitelist
			users.PhoneCountryWhitelist = map[string]string{
				"Japan": "81",
			}
			defer func() {
				users.PhoneCountryWhitelist = orig
			}()

			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateNoCard))
		})

		It("does not change a paid user's billing profile", func() {
			// Set user's billing profile to paid.
			_, err := db.Exec("UPDATE billing_profiles SET state = $1 WHERE user_id = $2;", billingprofile.StatePaid, u.ID)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("code", v.Code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaid))
		})

		It("responds with HTTP 400 Bad Request if verification code is wrong", func() {
			code := "WRONGCODE!"
			Expect(code).NotTo(Equal(v.Code))
			params := &url.Values{}
			params.Set("code", code)
			doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
			Expect(errDesc).To(Equal("code is incorrect"))
		})

		It("responds with HTTP 400 Bad Request when code is not given", func() {
			doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
		})

		It("responds with HTTP 400 Bad Request if user has not given us his phone number", func() {
			// Create another user that does not have a phone in his record.
			u = &user.User{
				Username: "anotheruser",
				Password: "123456",
				Email:    "anotheruser@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			t = &token.Token{
				UserID: u.ID,
				Token:  "anothertoken",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("code", "SOMECODE")
			doRequest(params)

			doRequest(nil)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
		})
	})

	Describe("GET /api/v0/user", func() {
		doRequest := func(token string) {
			var err error
			req, err = test.NewRequest("GET", "/api/v0/user", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			var (
				userinfo map[string]interface{}
			)

			ItReturnsUserDetails := func() {
				It("returns user details", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					var ok bool
					userinfo, ok = j["user"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(userinfo).NotTo(BeNil())
					Expect(userinfo["username"]).To(Equal("tokenuser"))
					Expect(userinfo["email"]).To(Equal("tokenuser@example.com"))
					Expect(userinfo["created_at"]).NotTo(BeNil())
					Expect(userinfo["updated_at"]).NotTo(BeNil())
					Expect(userinfo["password"]).To(BeNil())
					Expect(userinfo["encrypted_password"]).To(BeNil())

					userprofile, ok := userinfo["user_profile"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(userprofile["advanced"]).NotTo(BeTrue())
				})
			}

			Context("when user has billing issues", func() {
				BeforeEach(func() {
					test.MarkDelinquentUser(db, u)
					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()
			})

			Context("when user doesn't have billing issues", func() {
				BeforeEach(func() {
					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()

				It("responds with state", func() {
					billingprofile, ok := userinfo["billing_profile"].(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(billingprofile["state"]).To(Equal("no_card"))
				})
			})

			Describe("when user is paid", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StatePaid, u.ID)
					Expect(err).To(BeNil())

					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()

				It("should indicate if the user can use the desktop app", func() {
					Expect(userinfo["can_use_desktop_app"]).To(BeTrue())
				})
			})

			Describe("when user is free", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
					Expect(err).To(BeNil())

					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()

				It("should indicate if the user can use the desktop app", func() {
					Expect(userinfo["can_use_desktop_app"]).To(BeFalse())
				})
			})

			Describe("when user is member of paid org", func() {
				BeforeEach(func() {
					o := &org.Org{
						Slug:  "Foobar",
						Name:  "Foo Bar, Inc.",
						Email: "eng@foobar.com",
					}
					err := o.Insert(db)
					Expect(err).To(BeNil())
					err = db.WithTransaction(func(tx *sqlx.Tx) error {
						return billingprofile.Create(tx, u.ID, sql.NullInt64{o.ID, true})
					})
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateFree, u.ID)
					Expect(err).To(BeNil())

					_, err = db.Exec("INSERT INTO org_memberships (org_id, user_id, role) values ($1, $2, $3);", o.ID, u.ID, "member")
					Expect(err).To(BeNil())

					_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2;`, billingprofile.StatePaid, o.ID)
					Expect(err).To(BeNil())

					doRequest("foobarbazqux")
				})

				ItReturnsUserDetails()

				It("should indicate if the user can use the desktop app", func() {
					Expect(userinfo["can_use_desktop_app"]).To(BeTrue())
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/user/login_token", func() {
		doRequest := func(token string) {
			var err error
			req, err = test.NewRequest("GET", "/api/v0/user/login_token", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			BeforeEach(func() {
				doRequest("foobarbazqux")
			})

			It("returns a valid login token", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				var ok bool
				var tok string
				tok, ok = j["login_token"].(string)
				Expect(ok).To(BeTrue())
				Expect(tok).NotTo(BeEmpty())

				id, err := logintoken.Read(tok)
				Expect(err).To(BeNil())
				Expect(id).To(Equal(u.ID))
			})
		})

		Context("when a non-valid oauth token is given", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user/login_token"
				authTokenInputs.Method = "GET"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/user/confirm", func() {
		var (
			u                 *user.User
			confirmationToken = "a1b2c3d4"
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = $2 WHERE id = $3;`, time.Now().Add(-48*time.Hour), confirmationToken, u.ID)
			Expect(err).To(BeNil())
		})

		doRequest := func(token string) {
			var err error
			params := &url.Values{}
			params.Set("token", token)
			req, err = test.NewRequest("POST", "/api/v0/user/confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return a bad request when the token is not provided", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("validation_failed"))
			Expect(j["error_description"]).To(Equal("`token` must be provided"))
		})

		It("should error out if the token is incorrect", func() {
			doRequest("incorrect-token")

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("validation_failed"))
			Expect(errDesc).To(Equal("specified user does not exist"))
		})

		It("should successfully return a token for the user", func() {
			Expect(u.ConfirmedAt.Valid).To(BeFalse(), "expect user to not confirmed yet")
			doRequest(confirmationToken)

			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["access_token"]).ToNot(BeNil())
			Expect(j["token_type"]).ToNot(BeNil())

			t, err := token.FindByToken(db, j["access_token"].(string))
			Expect(err).To(BeNil())
			Expect(t).ToNot(BeNil())
			Expect(t.UserID).To(Equal(u.ID))

			// and track the confirmation on mixpanel
			Expect(ft.PropsCalled).To(HaveLen(2))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(u.ID))
			Expect(pc.Mode).To(Equal("setnx"))
			ctime, ok := pc.PropsSet["confirmed_email_date"]
			Expect(ok).To(BeTrue())
			pqt, ok := ctime.(time.Time)
			Expect(ok).To(BeTrue())
			Expect(pqt).To(BeTemporally("~", time.Now(), time.Minute))

			// and update email
			pc2 := ft.PropsCalled[1]
			Expect(pc2.UserID).To(Equal(u.ID))
			Expect(pc2.Mode).To(Equal("set"))
			Expect(pc2.PropsSet["$email"]).To(Equal(u.Email))
		})

		It("sets the user's billing profile to free", func() {
			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateNoCard))

			doRequest(confirmationToken)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StateFree))
		})

		It("does not set the user's billing profile to free if it's already paid", func() {
			// Set user's billing profile to paid.
			bp, err := billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			bp.State = billingprofile.StatePaid
			err = bp.Update(db)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaid))

			doRequest(confirmationToken)

			Expect(res.Code).To(Equal(http.StatusOK))

			bp, err = billingprofile.Find(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bp.State).To(Equal(billingprofile.StatePaid))
		})

		It("should communicate if the confirmation has lapsed", func() {
			_, err := db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = $2 WHERE id = $3;`, time.Now().Add(-73*time.Hour), confirmationToken, u.ID)
			Expect(err).To(BeNil())

			doRequest(confirmationToken)

			Expect(res.Code).To(Equal(http.StatusBadRequest))
			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("confirmation_expired"))
			Expect(errDesc).To(Equal(user.ErrConfirmationLapsed.Error()))
		})
	})

	Describe("POST /api/v0/user/resend_confirm", func() {
		doRequest := func(token, appToken string) {
			var err error
			params := &url.Values{}
			params.Set("token", token)
			params.Set("app_token", appToken)
			req, err = test.NewRequest("POST", "/api/v0/user/resend_confirm", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return a bad request when the token is not provided", func() {
			doRequest("", "")
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["error"]).To(Equal("validation_failed"))
			Expect(j["error_description"]).To(Equal("`token` must be provided"))
		})

		Context("with remaining confirmation token", func() {
			var u *user.User
			curTime := time.Now()

			BeforeEach(func() {
				u = &user.User{
					Username: "qux",
					Name:     "quxboy",
					Password: "123456",
					Email:    "qux@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET confirmation_sent_at = $1, confirmation_token = 'ABCD' WHERE id = $2;`, curTime.Add(-48*time.Hour), u.ID)
				Expect(err).To(BeNil())
			})

			It("should error out if the token is incorrect", func() {
				doRequest("sdas", "")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the user has been deleted", func() {
				_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("sdas", "")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should send the confirmation token if the token matches", func() {
				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("ABCD", "")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["confirmation_sent"]).To(BeTrue())

				u, err = user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
				Expect(u.ConfirmationToken.String).ToNot(Equal("ABCD"))
				Expect(u.ConfirmationSentAt.Valid).To(BeTrue(), "expect confirmation to be sent")
			})
		})

		Context("with user's app token", func() {
			var u *user.User
			var t *token.Token

			BeforeEach(func() {
				u = &user.User{
					Username: "qux",
					Name:     "quxboy",
					Password: "123456",
					Email:    "qux@example.com",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				t = &token.Token{
					UserID: u.ID,
					Token:  "toke",
				}

				err = t.Insert(db)
				Expect(err).To(BeNil())
			})

			It("should error out if the token is incorrect", func() {
				doRequest("", "tokey")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the token is invalid", func() {
				_, err := db.Exec(`UPDATE tokens SET last_seen_at = now() - interval '14 days' WHERE token = 'toke';`)
				Expect(err).To(BeNil())

				doRequest("", "toke")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should error out if the user has been deleted", func() {
				_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, u.ID)
				Expect(err).To(BeNil())

				doRequest("", "toke")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("specified user does not exist"))
			})

			It("should send the confirmation token if the token matches", func() {
				Expect(u.ConfirmationSentAt.Valid).To(BeFalse(), "expect confirmation to not be sent")
				doRequest("", "toke")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(BeNil())
				Expect(j["confirmation_sent"]).To(BeTrue())

				u, err = user.FindByID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(u.ConfirmationToken.String).ToNot(BeEmpty())
				Expect(u.ConfirmationToken.String).ToNot(Equal("ABCD"))
				Expect(u.ConfirmationSentAt.Valid).To(BeTrue(), "expect confirmation to be sent")
			})
		})
	})

	Describe("POST /api/v0/user/dismiss_first_time", func() {
		doRequest := func() {
			var err error
			req, err = test.NewRequest("POST", "/api/v0/user/dismiss_first_time", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)
			handler.ServeHTTP(res, req)
		}

		It("should set the first_time to FALSE", func() {
			Expect(u.FirstTime).To(BeTrue())

			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j["status"]).To(Equal("ok"))

			u, err = user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(u.FirstTime).To(BeFalse())
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user/dismiss_first_time"
				authTokenInputs.Method = "POST"
			})

			test.ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/user/exists", func() {
		var (
			client       *oauthclient.Client
			clientSecret = "abc123"
		)

		BeforeEach(func() {
			client = &oauthclient.Client{
				UserID:      u.ID,
				Name:        "Builders League United",
				Description: "A fictional company",
				ClientID:    "blu_team",
				RedirectURI: "https://blu.co/oauth/callback",
			}
			Expect(client.InsertWithSecret(db, clientSecret)).To(BeNil())
		})

		doRequest := func(email, clientID, clientSecret string) {
			var err error
			params := &url.Values{}
			if email != "" {
				params.Set("email", email)
			}
			if clientID != "" {
				params.Set("client_id", client.ClientID)
				params.Set("client_secret", clientSecret)
			}
			req, err = test.NewRequest("GET", "/api/v0/user/exists", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)
			handler.ServeHTTP(res, req)
		}

		It("should be 404 if user does not exist", func() {
			doRequest("nosuch@example.com", client.ClientID, clientSecret)
			Expect(res.Code).To(Equal(http.StatusNotFound))
		})

		It("should be 200 if user exists", func() {
			doRequest(u.Email, client.ClientID, clientSecret)
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("is only available to oauth clients", func() {
			// no oauth_client info
			doRequest(u.Email, "", "")
			Expect(res.Code).To(Equal(http.StatusForbidden))

			// invalid oauth_client info
			doRequest(u.Email, "notaclient", "abcdef")
			Expect(res.Code).To(Equal(http.StatusForbidden))
		})
	})
})
