package userkeys_test

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
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/userkey"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/userkeys")
}

var _ = Describe("controllers/userkeys", func() {
	validKeys := []string{
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn",
		"ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAEJfzWE4kBhqfyJV5PGUhCyaJ9GB0RRrc3RsCweBLgLmbagOy4IwRtd79/AgxlmoZ/7e8eTNfAhE4vV6MXJoltIkAFz+0+NsAopvQEWWDHfADilCuTltegQn6r1f6iA5XgSoLJQvnJVaneo4UzClCHIVDZWQL1XvNlKKtxRHU0q0niElA==",
	}

	var (
		db              *ndb.Database
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		params          *url.Values
		hs              *HostStack
		c               *container.Container
		authToken       *token.Token
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	BeforeEach(func() {
		var err error
		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
		})
		Expect(err).To(BeNil())

		authToken = &token.Token{
			UserID: hs.U.ID,
			Token:  "valid-token",
		}
		err = authToken.Insert(db)
		Expect(err).To(BeNil())

		c = &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
	})

	Describe("GET /api/v0/containers/{container}/keys", func() {
		doRequest := func(token string) {
			var err error
			req, err = NewAPIRequest(token, "GET", fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, hs.U)
			doRequest("valid-token")
			VerifyDelinquentUserResponse(res)
		})

		Context("when a valid oauth token is given", func() {
			Context("when user has keys", func() {
				var pks []*userkey.UserKey

				BeforeEach(func() {
					for _, k := range validKeys {
						pk := &userkey.UserKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         k,
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())
						pks = append(pks, pk)
					}
				})

				It("responds with json keys", func() {
					params = &url.Values{}
					doRequest("valid-token")

					Expect(res.Code).To(Equal(http.StatusOK))
					j := struct {
						Keys []*serializers.UserKey `json:"keys"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					Expect(j.Keys).To(HaveLen(2))
					for _, k := range j.Keys {
						Expect(validKeys).To(ContainElement(k.Key))
						Expect(k.ContainerID).To(Equal(c.ID))
					}
				})
			})

			Context("when user has no keys", func() {
				It("returns empty array", func() {
					params = &url.Values{}
					doRequest("valid-token")

					Expect(res.Code).To(Equal(http.StatusOK))
					j := struct {
						Keys []*serializers.UserKey `json:"keys"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					Expect(j.Keys).To(BeEmpty())
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	for _, method := range []string{"POST", "PUT"} {
		m := method
		Describe(m+" /api/v0/containers/{container}/keys", func() {
			doRequest := func(token, slug string) {
				var err error
				req, err = NewAPIRequest(token, m, fmt.Sprintf("/api/v0/containers/%s/keys", slug), params)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)
			}

			It("should block if the user has billing issues", func() {
				MarkDelinquentUser(db, hs.U)
				doRequest("valid-token", c.Slug)
				VerifyDelinquentUserResponse(res)
			})

			It("should return an internal server error if no billing profile records", func() {
				_, err := db.Exec("DELETE FROM billing_profiles WHERE user_id = $1;", hs.U.ID)
				Expect(err).To(BeNil())

				doRequest("valid-token", c.Slug)
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			Context("when valid oauth token is present", func() {
				Context("when valid params are received", func() {
					BeforeEach(func() {
						params = &url.Values{}
						params.Set("key", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0Ddg2BE+XhqJHHI164Sxi7euv5bTTxdW1D/mUTd1uQruFvyyMPAVjRhR1aSgvh8DGlaj/ndV5CViR6zU9upLf9XPjCg84/4HMBpbhSA3PWRb6YMwf85HEAfiXYLmw06PD40Ts6EHbvtWfQLo06bxsnGaRaP4dJNZRkDpEuQTZNoWh0SQzG3Ww5TA7V80WCAdBnIOXymjGDm+Ri24RibmO+mp4s2dnevZOPXAOSAn5axd5X6INg+TEYFaEvFT8JnBTBvCWTTeHj+Xti9wWBcGbdFI/vAoHNrDtCav0UBe5ueItXAUklm4lo7uG+CL/33fLQ+iEC+WTTivTt9U0IORB")
						mockClient.AddUserKeysError = nil
					})

					It("creates the key", func() {
						doRequest("valid-token", c.Slug)

						Expect(mockClient.AddUserKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))
						pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(HaveLen(1))

						Expect(res.Code).To(Equal(http.StatusCreated))
						j := map[string]string{}
						err = json.NewDecoder(res.Body).Decode(&j)
						Expect(err).To(BeNil())
						Expect(j["status"]).To(Equal("created"))
					})
				})

				Context("when key already exists", func() {
					BeforeEach(func() {
						pk := &userkey.UserKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         validKeys[0],
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())
					})

					It("does not return an error", func() {
						params = &url.Values{}
						params.Set("key", validKeys[0])
						doRequest("valid-token", c.Slug)

						Expect(res.Code).To(Equal(http.StatusCreated))
						j := map[string]string{}
						err := json.NewDecoder(res.Body).Decode(&j)
						Expect(err).To(BeNil())
						Expect(j["status"]).To(Equal("created"))
					})
				})

				Context("when invalid key is received", func() {
					It("responds with bad request", func() {
						params = &url.Values{}
						params.Set("key", "ssh-rsa bad key!!")
						doRequest("valid-token", c.Slug)

						Expect(res.Code).To(Equal(http.StatusBadRequest))
						_, errDesc := ErrorSlug(res.Body)
						Expect(errDesc).To(Equal("invalid key"))
					})
				})

				Context("when invalid container id is received", func() {
					It("responds with bad request", func() {
						params = &url.Values{}
						params.Set("key", validKeys[0])
						doRequest("valid-token", "container-that-does-not-exist")

						Expect(res.Code).To(Equal(http.StatusNotFound))
						_, errDesc := ErrorSlug(res.Body)
						Expect(errDesc).To(Equal("container does not exist"))
					})
				})

				Context("when the call to freighter fails for some reason", func() {
					BeforeEach(func() {
						mockClient.AddUserKeysError = freighterclient.ErrPubKeyOpsNotSupported
						params = &url.Values{}
						params.Set("key", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0Ddg2BE+XhqJHHI164Sxi7euv5bTTxdW1D/mUTd1uQruFvyyMPAVjRhR1aSgvh8DGlaj/ndV5CViR6zU9upLf9XPjCg84/4HMBpbhSA3PWRb6YMwf85HEAfiXYLmw06PD40Ts6EHbvtWfQLo06bxsnGaRaP4dJNZRkDpEuQTZNoWh0SQzG3Ww5TA7V80WCAdBnIOXymjGDm+Ri24RibmO+mp4s2dnevZOPXAOSAn5axd5X6INg+TEYFaEvFT8JnBTBvCWTTeHj+Xti9wWBcGbdFI/vAoHNrDtCav0UBe5ueItXAUklm4lo7uG+CL/33fLQ+iEC+WTTivTt9U0IORB")
					})

					It("responds with bad request", func() {
						doRequest("valid-token", c.Slug)

						Expect(res.Code).To(Equal(http.StatusBadRequest))
						_, errDesc := ErrorSlug(res.Body)
						Expect(errDesc).To(Equal("public key operations not supported on this container"))
					})

					It("should not persist the key in the database", func() {
						doRequest("valid-token", c.Slug)

						pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(HaveLen(0))
					})
				})
			})

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
					authTokenInputs.Method = m
				})

				ItRequiresAuthToken(&authTokenInputs)
			})

		})
	}

	Describe("DELETE /api/v0/containers/{container}/keys", func() {
		doRequest := func(token, slug string) {
			var err error
			req, err = NewAPIRequest(token, "DELETE", fmt.Sprintf("/api/v0/containers/%s/keys", slug), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, hs.U)
			doRequest("valid-token", c.Slug)
			VerifyDelinquentUserResponse(res)
		})

		It("should return an internal server error if no billing profile records", func() {
			_, err := db.Exec("DELETE FROM billing_profiles WHERE user_id = $1;", hs.U.ID)
			Expect(err).To(BeNil())

			doRequest("valid-token", c.Slug)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})

		Context("with valid oauth token", func() {
			Context("when user has multiple keys", func() {
				BeforeEach(func() {
					for i := 0; i < 2; i++ {
						pk := &userkey.UserKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         validKeys[i],
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())
					}

					pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
					Expect(err).To(BeNil())
					Expect(pks).To(HaveLen(2))

					params = &url.Values{}
				})

				Context("when call to freighterclient succeeds", func() {
					It("removes all of them", func() {
						mockClient.ClearUserKeysError = nil
						doRequest("valid-token", c.Slug)

						Expect(mockClient.ClearUserKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))
						pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(BeEmpty())

						Expect(res.Code).To(Equal(http.StatusOK))
						j := map[string]string{}
						err = json.NewDecoder(res.Body).Decode(&j)
						Expect(err).To(BeNil())
						Expect(j["status"]).To(Equal("ok"))
					})
				})

				Context("when the call to freighter fails for some reason", func() {
					BeforeEach(func() {
						mockClient.ClearUserKeysError = freighterclient.ErrPubKeyOpsNotSupported
					})

					It("returns bad request", func() {
						doRequest("valid-token", c.Slug)

						Expect(mockClient.ClearUserKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))

						Expect(res.Code).To(Equal(http.StatusBadRequest))
						_, errDesc := ErrorSlug(res.Body)
						Expect(errDesc).To(Equal("public key operations not supported on this container"))
					})

					It("should not destroy the keys from the database", func() {
						doRequest("valid-token", c.Slug)

						pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(len(pks)).To(Equal(2))
					})
				})
			})

			Context("when user has no keys", func() {
				It("returns success", func() {
					params = &url.Values{}
					doRequest("valid-token", c.Slug)
					mockClient.ClearUserKeysError = nil

					Expect(mockClient.ClearUserKeysCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))

					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]string{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					Expect(j["status"]).To(Equal("ok"))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/keys/<kslug>", func() {
		var pks []*userkey.UserKey
		var c2 *container.Container
		var fprint string

		doRequest := func(token, slug, identifier string) {
			var err error
			req, err = NewAPIRequest(token, "DELETE",
				fmt.Sprintf("/api/v0/containers/%s/keys/%s", slug, identifier), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			pks = []*userkey.UserKey{}
			for i := 0; i < 2; i++ {
				pk := &userkey.UserKey{
					UserID:      hs.U.ID,
					ContainerID: c.ID,
					Key:         validKeys[i],
				}
				err := pk.Insert(db)
				Expect(err).To(BeNil())
				pks = append(pks, pk)
			}

			c2 = &container.Container{
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "go-benchmark",
			}
			err := c2.Insert(db)
			Expect(err).To(BeNil())
			fprint, err = pks[1].Md5Fingerprint()
			Expect(err).To(BeNil())
			mockClient.RemoveUserKeyCalled = 0
		})

		It("should block if the user has billing issues", func() {
			MarkDelinquentUser(db, hs.U)
			doRequest("valid-token", c.Slug, fprint)
			VerifyDelinquentUserResponse(res)
		})

		It("should return an internal server error if no billing profile records", func() {
			_, err := db.Exec("DELETE FROM billing_profiles WHERE user_id = $1;", hs.U.ID)
			Expect(err).To(BeNil())

			doRequest("valid-token", c.Slug, fprint)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
		})

		Context("with valid oauth token", func() {
			Context("when call to freighter is successful", func() {
				It("is successful when key with given fingerprint is present in container", func() {
					mockClient.RemoveUserKeyError = nil
					doRequest("valid-token", c.Slug, fprint)

					Expect(mockClient.RemoveUserKeyCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))

					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]string{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					Expect(j["status"]).To(Equal("ok"))
				})

				It("is successful when key with given ID is present in container", func() {
					mockClient.RemoveUserKeyError = nil
					doRequest("valid-token", c.Slug, strconv.FormatInt(pks[1].ID, 10))

					Expect(mockClient.RemoveUserKeyCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))

					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when call to freighter is unsuccessful", func() {
				BeforeEach(func() {
					mockClient.RemoveUserKeyError = freighterclient.ErrPubKeyOpsNotSupported
				})

				It("should respond with bad request", func() {
					doRequest("valid-token", c.Slug, fprint)

					Expect(mockClient.RemoveUserKeyCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					_, errDesc := ErrorSlug(res.Body)
					Expect(errDesc).To(Equal("public key operations not supported on this container"))
				})

				It("should not destroy the keys from the database", func() {
					doRequest("valid-token", c.Slug, fprint)

					pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
					Expect(err).To(BeNil())
					Expect(len(pks)).To(Equal(2))
				})
			})

			It("is unsuccessful when key is not in container", func() {
				doRequest("valid-token", c2.Slug, fprint)
				Expect(mockClient.RemoveUserKeyCalled).To(Equal(0))

				Expect(res.Code).To(Equal(http.StatusNotFound))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("fingerprint not found"))

				pks, err := userkey.FindAll(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(len(pks)).To(Equal(2))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys/%s", c.Slug, fprint)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
