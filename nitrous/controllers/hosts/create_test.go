package hosts_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/hosts"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("POST /api/v0/hosts", func() {
	var (
		db              *ndb.Database
		svr             *server.Server
		handler         *mux.Router
		authTokenInputs test.AuthTokenInputs
		params          *url.Values

		mockClient *freighterclient.MockClient

		u     *user.User
		t     *token.Token
		ft    *test.FakeTracker
		hs    *test.HostStack
		orgHs *test.HostStack
		o     *org.Org

		// save the original hostbuilderclient value to be restored later
		origHostbuilderClient = *hosts.HostbuilderClient
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		test.StubDefaultFreighterFn(mockClient)

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		ft = test.NewFakeTracker()
		svr = test.NewTestServer("", db, ft)
		handler = svr.Router()
		authTokenInputs = test.AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		_, err = db.Exec(`UPDATE billing_profiles SET state = 'paid' WHERE user_id = $1;`, u.ID)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		// Replace global hostbuilderclient to make tests consistent
		// this client is shared between hosts and provisioner
		// FIXME: find a way to not depend on global variable too much
		hb := hostbuilderclient.NewHostbuilder(envvars.HostBuilderURL)
		replaceTransport(hb, &test.FakeRoundTripper{Wait: 1 * time.Second})
		hosts.HostbuilderClient = hb
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		test.UnstubFreighterFn()
		// restore the original hostbuilder client
		hosts.HostbuilderClient = &origHostbuilderClient
	})

	BeforeEach(func() {
		var err error

		hs, err = test.CreateHostStack(db, &test.HostStack{
			R: &region.Region{Slug: "use"},
			P: &plan.Plan{Name: "Large", Slug: "large", Price: 500},
		})
		Expect(err).To(BeNil())

		orgHs, err = test.CreateHostStack(db, &test.HostStack{
			R: &region.Region{Slug: "use2"},
			P: &plan.Plan{Name: "OrgLarge", Slug: "org-large", Price: 10000, Org: true},
		})
		Expect(err).To(BeNil())

		o = &org.Org{
			Name:  "Foo Bar, Inc.",
			Slug:  "foobar",
			Email: "eng@foobar.com",
		}
		err = o.Insert(db)
		Expect(err).To(BeNil())

		params = &url.Values{}
		params.Set("name", "rails-host")
		params.Set("size", hs.P.Slug)
		params.Set("region", hs.R.Slug)
	})

	type overrides map[string]string
	doRequest := func(p *url.Values) *httptest.ResponseRecorder {
		req, err := test.NewRequest("POST", "/api/v0/hosts", p)
		Expect(err).To(BeNil())
		req.Header.Set("Authorization", "Bearer valid-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	doJsonRequest := func(jsonStr string) *httptest.ResponseRecorder {
		req, err := test.NewBodyRequest("POST", "/api/v0/hosts", jsonStr)
		Expect(err).To(BeNil())
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-type", "application/json")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	It("should block if the user has billing issues", func() {
		test.MarkDelinquentUser(db, u)
		res := doRequest(params)
		test.VerifyDelinquentUserResponse(res)
	})

	It("should return vague error if user is suspicious and plan is free", func() {
		_, err := db.Exec(`
			UPDATE users SET
				suspicious = 'bad email',
				phone_authed = true
			WHERE id = $1
		`, u.ID)
		Expect(err).To(BeNil())

		_, err = db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2`, billingprofile.StateFree, u.ID)
		Expect(err).To(BeNil())

		_, err = db.Exec("UPDATE plans SET price = 0 WHERE id = $1", hs.P.ID)
		Expect(err).To(BeNil())

		res := doRequest(params)

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("Unexpected error"))
	})

	// tests for required params
	for _, param := range []string{"size", "region", "name"} {
		p := param

		It("responds with "+p+" required when request does not specify a "+p, func() {
			params.Del(p)
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal(p + " is required"))
		})
	}

	It("responds with name is required when an empty name is specified", func() {
		params.Set("name", "")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("name is required"))
	})

	It("should not send any analytics events if there are errors", func() {
		params.Set("name", "")
		doRequest(params)
		t, ok := svr.Tracker.(*test.FakeTracker)
		Expect(ok).To(BeTrue())

		Expect(t.Events[u.ID]).To(BeEmpty())
	})

	It("responds with plan not found when a matching plan cannot be found for the specified size", func() {
		params.Set("size", "doubledouble")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching plan found"))
	})

	It("responds with plan not found when the requested plan is disabled", func() {
		planSlug := "3gb"
		p := &plan.Plan{Disabled: true, Slug: planSlug, Name: "3gb", MaxContainers: 1, Price: 500}
		Expect(p.Insert(db)).To(BeNil())

		params.Set("size", planSlug)
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching plan found"))
	})

	Context("with an org", func() {
		BeforeEach(func() {
			params.Set("size", orgHs.P.Slug)
			params.Set("region", orgHs.R.Slug)
			params.Set("org", o.Slug)

			// mimic user being fresh
			_, err := db.Exec("UPDATE billing_profiles SET state = 'no_card' WHERE user_id = $1", u.ID)
			Expect(err).To(BeNil())
		})

		It("responds with an error if the org could not be found", func() {
			params.Set("org", "someorg")
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("No matching org found"))
		})

		It("responds with an error if org is specified and plan selected is not an org plan", func() {
			err := db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())

			params.Set("size", hs.P.Slug)
			params.Set("region", hs.R.Slug)
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("Not an org plan"))
		})

		It("responds with an error if the user is not a member of the org", func() {
			err := db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("org_forbidden"))
			Expect(errDesc).To(Equal("You do not have access to this organization"))
		})

		It("responds with an error if the user is a limited member of the org", func() {
			err := db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})).To(BeNil())
				_, err := orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleLimited)
				return err
			})
			Expect(err).To(BeNil())
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("org_forbidden"))
			Expect(errDesc).To(Equal("You do not have permission to create workstations under this organization"))
		})
	})

	It("responds with region disabled when a disabled region is specified", func() {
		_, err := db.Exec("INSERT INTO regions (slug, name, rank, disabled_at) values ($1, $2, $3, now());", "newfoundland", "New Found Land", 1)
		Expect(err).To(BeNil())
		params.Set("region", "newfoundland")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("region_disabled"))
		Expect(errDesc).To(Equal("region is disabled"))
	})

	It("responds with region not found when an invalid region is specified", func() {
		params.Set("region", "antarctica")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching region found for the given name"))
	})

	It("responds with provider template not found error when no provider template found for the given plan and region", func() {
		var r2 *region.Region

		r2 = &region.Region{Slug: "usw", Name: "US West"}
		err := r2.Insert(db)
		Expect(err).To(BeNil())

		params.Set("region", r2.Slug)
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching templates found for the given plan and region combination"))
	})

	It("sends the host configs as json params", func() {
		hb := hostbuilderclient.NewHostbuilder("www.example.com")
		rt := &test.FakeRoundTripper{
			Message: fmt.Sprintf(`
			{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
			`, hs.PT.Name),
			Status: http.StatusOK,
		}
		replaceTransport(hb, rt)
		hosts.HostbuilderClient = hb
		doRequest(params)

		req := rt.Requests[0]
		Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())

		var config hostbuilderclient.Config
		json.Unmarshal([]byte(req.FormValue("config")), &config)
		Expect(config["name"]).To(HavePrefix(params.Get("name")), "invalid slug set in name")
		Expect(slugbuilder.IsValid(config["name"].(string))).To(BeTrue(), "invalid slug set in name")

		// Test that we use the host ID and secret as Apt user/pass.
		hosts, err := host.FindAllByUser(db, u.ID)
		Expect(err).To(BeNil())
		Expect(hosts).To(HaveLen(1))
		h := hosts[0]
		Expect(config["apt_username"]).To(Equal(strconv.FormatInt(h.ID, 10)))
		Expect(config["apt_password"]).To(Equal(h.Secret))
	})

	It("sends the host configs as json params when JSON is sent by the client", func() {
		jsonStr := `{
			"host": {
				"slug": "sLug1",
				"name": "MyHost",
				"region": "use",
				"size": "large"
			}
		}`

		hb := hostbuilderclient.NewHostbuilder("www.example.com")
		rt := &test.FakeRoundTripper{
			Message: fmt.Sprintf(`
			{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
			`, hs.PT.Name),
			Status: http.StatusOK,
		}
		replaceTransport(hb, rt)
		hosts.HostbuilderClient = hb
		doJsonRequest(jsonStr)

		Expect(rt.Requests).To(HaveLen(1))
		req := rt.Requests[0]
		var config hostbuilderclient.Config
		json.Unmarshal([]byte(req.FormValue("config")), &config)
		Expect(config["name"]).To(HavePrefix("myhost"), "invalid slug set in name")
		Expect(slugbuilder.IsValid(config["name"].(string))).To(BeTrue(), "invalid slug set in name")

		// Test that we use the host ID and secret as Apt user/pass.
		hosts, err := host.FindAllByUser(db, u.ID)
		Expect(err).To(BeNil())
		Expect(hosts).To(HaveLen(1))
		h := hosts[0]
		Expect(config["apt_username"]).To(Equal(strconv.FormatInt(h.ID, 10)))
		Expect(config["apt_password"]).To(Equal(h.Secret))
	})

	It("should ignore if the promocode is unavailable", func() {
		params.Set("promo_code", "just10")
		hb := hostbuilderclient.NewHostbuilder("www.example.com")
		rt := &test.FakeRoundTripper{
			Message: fmt.Sprintf(`
			{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
			`, hs.PT.Name),
			Status: http.StatusOK,
		}
		replaceTransport(hb, rt)
		hosts.HostbuilderClient = hb
		res := doRequest(params)
		Expect(res.Code).To(Equal(http.StatusOK))
	})

	It("should create a claim for a promocode if supplied", func() {
		var pcID int64
		err := db.Get(&pcID, "INSERT INTO promo_codes(slug, quantity, trial_seconds, expires_at) VALUES ('just10', 10, 900, now()+interval '2 days') RETURNING id;")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO promo_code_plans(promo_code_id, plan_id) VALUES ($1, $2);", pcID, hs.P.ID)
		Expect(err).To(BeNil())

		params.Set("promo_code", "just10")
		hb := hostbuilderclient.NewHostbuilder("www.example.com")
		rt := &test.FakeRoundTripper{
			Message: fmt.Sprintf(`
			{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
			`, hs.PT.Name),
			Status: http.StatusOK,
		}
		replaceTransport(hb, rt)
		hosts.HostbuilderClient = hb
		res := doRequest(params)
		Expect(res.Code).To(Equal(http.StatusOK))

		cl, err := promocode.FindPendingClaim(db, u.ID, hs.P.ID)
		Expect(err).To(BeNil())
		Expect(cl.Code.ID).To(Equal(pcID))
	})

	Context("when hostbuilder successfully creates a host", func() {
		var rt *test.FakeRoundTripper

		BeforeEach(func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt = &test.FakeRoundTripper{
				Message: fmt.Sprintf(`
				{"stream":"starting: create the droplet"}
				{"stream":"finished: create the droplet"}
				{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
				`, hs.PT.Name),
				Status: http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb
		})

		It("creates a host record for the user", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.RegionID).To(Equal(hs.R.ID))
			Expect(h.PlanID).To(Equal(hs.P.ID))
			Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

			Expect(h.Slug).To(HavePrefix(params.Get("name")))
			Expect(slugbuilder.IsValid(h.Slug)).To(BeTrue())
			Expect(h.Name.String).To(Equal(params.Get("name")))

			Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
			Expect(h.Size.String).To(Equal(hs.P.Slug))

			// and tracks on mixpanel
			Expect(ft.PropsCalled).To(HaveLen(1))
			pc := ft.PropsCalled[0]
			Expect(pc.UserID).To(Equal(h.UserID))
			Expect(pc.Mode).To(Equal("set"))
			Expect(pc.PropsSet["workspaces_plans"]).To(Equal([]string{"Large"}))
		})

		It("sets auto_shutdown true if the plan specifies it", func() {
			_, err := db.Exec("UPDATE plans SET auto_shutdown = true WHERE id = $1", hs.P.ID)
			Expect(err).To(BeNil())

			doRequest(params)
			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.AutoShutdown).To(BeTrue())
		})

		Context("if org is specified", func() {
			It("sets the org_id on the host", func() {
				err := db.WithTransaction(func(t *sqlx.Tx) error {
					return billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})
				})
				Expect(err).To(BeNil())
				_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
				Expect(err).To(BeNil())

				bp, err := billingprofile.FindForOrg(db, o.ID)
				Expect(err).To(BeNil())
				bp.State = billingprofile.StatePaid
				Expect(bp.Update(db)).To(BeNil())

				params.Set("org", o.Slug)
				params.Set("size", orgHs.P.Slug)
				params.Set("region", orgHs.R.Slug)

				res := doRequest(params)

				Expect(res.Code).To(Equal(http.StatusOK))

				hosts, err := host.FindAllByUser(db, u.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))

				h := hosts[0]
				Expect(h.OrgID.Int64).To(Equal(o.ID))
			})
		})

		It("creates a host record for the user when JSON is sent by the client", func() {
			jsonStr := `{
				"host": {
					"name": "MyHost",
					"region": "use",
					"size": "large"
				}
			}`
			doJsonRequest(jsonStr)

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.RegionID).To(Equal(hs.R.ID))
			Expect(h.PlanID).To(Equal(hs.P.ID))
			Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

			Expect(h.Slug).To(HavePrefix("myhost"))
			Expect(slugbuilder.IsValid(h.Slug)).To(BeTrue())
			Expect(h.Name.String).To(Equal("MyHost"))

			Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
			Expect(h.Size.String).To(Equal(hs.P.Slug))
		})

		It("creates a keypair associated with that host", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			kp, err := keypairs.Get(db, hosts[0].ID)
			Expect(err).To(BeNil())
			Expect(kp.HostID).To(Equal(hosts[0].ID))

			req := rt.Requests[0]
			var config hostbuilderclient.Config
			json.Unmarshal([]byte(req.FormValue("config")), &config)
			public_keys := config["public_keys"].([]interface{})
			Expect(public_keys[0].(string)).To(Equal(kp.PublicKey))
		})

		It("creates a shared secret for the host", func() {
			doRequest(params)
			hosts, _ := host.FindAllByUser(db, u.ID)

			Expect(hosts[0].Secret).NotTo(BeEmpty())
			req := rt.Requests[0]
			var config hostbuilderclient.Config
			json.Unmarshal([]byte(req.FormValue("config")), &config)
			Expect(config["secret"]).To(Equal(hosts[0].Secret))
		})

		It("creates a cert associated with that host", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			crt, err := cert.Find(db, hosts[0].ID)
			Expect(err).To(BeNil())
			Expect(crt.HostID).To(Equal(hosts[0].ID))
			Expect(crt.ServerCert).To(ContainSubstring("BEGIN CERTIFICATE"))
			Expect(crt.ServerKey).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		})

		It("responds with the progress data and created host", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))
			buf := bytes.NewBuffer(nil)
			io.Copy(buf, res.Body)

			Expect(res.Header().Get("X-Accel-Buffering")).To(Equal("no"), "failed to disable buffering in nginx")
			Expect(buf.String()).To(ContainSubstring("starting: create the droplet"), "expect response to include hostbuilder progress")

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			decoder := json.NewDecoder(buf)
			for {
				err := decoder.Decode(&j)
				if j.Host != nil {
					break
				}
				if err == io.EOF && j.Host == nil {
					Fail("invalid response, missing host info")
				}
			}

			hosts, _ := host.FindAllByUser(db, u.ID)
			Expect(j.Host.ID).To(Equal(hosts[0].ID))
			Expect(j.Host.UserID).To(Equal(u.ID))
			Expect(j.Host.RegionID).To(Equal(hs.R.ID))
			Expect(j.Host.PlanID).To(Equal(hs.P.ID))
			Expect(j.Host.Slug).To(HavePrefix(params.Get("name")))
			Expect(slugbuilder.IsValid(j.Host.Slug)).To(BeTrue())
			Expect(j.Host.Size).To(Equal(hs.P.Slug))
			Expect(j.Host.Hostname).To(Equal(hostname(j.Host.Slug))) // can use the host slug here, since it's already tested above
			Expect(j.Host.CreatedAt).NotTo(BeNil())
			Expect(j.Host.UpdatedAt).NotTo(BeNil())
			Expect(j.Host.TerminatedAt).To(BeNil())
		})

		It("publishes events for the created host", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest(params)

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			h := hosts[0]

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail("timedout while waiting for events")
			case <-finished:
				Expect(msgs[0]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{Slug: h.Slug,
						Progress: `{"stream":"starting: create the droplet"}`},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[1]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{Slug: h.Slug,
						Progress: `{"stream":"finished: create the droplet"}`},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[2]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{Slug: h.Slug,
						Progress: fmt.Sprintf(
							`{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}`,
							hs.PT.Name,
						)},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[3]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreated,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(h),
					},
				}), "unexpected event, wanted EventHostCreated")
			}
		})

		It("converts the 'name' form value to valid slug", func() {
			params.Set("name", "Rails-Host")
			doRequest(params)

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].Slug).To(HavePrefix("rails-host"))
			Expect(slugbuilder.IsValid(hosts[0].Slug)).To(BeTrue())
		})

		It("does not create the host when there's a host creation going on", func() {
			tx, _ := db.Beginx()
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(bp.ID))
			defer tx.Rollback()

			params.Set("name", "host-unique")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusConflict))
		})

		It("ignores provider param if provided by non-admin", func() {
			params.Set("provider", "dummy")
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ProviderTemplateID).To(Equal(hs.PT.ID))
		})

		It("uses provider param if provided by admin", func() {
			pt2 := &providertemplate.ProviderTemplate{
				Name:     "default-1",
				Provider: "dummy",
				RegionID: hs.PT.RegionID,
				PlanID:   hs.PT.PlanID,
			}
			err := pt2.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET admin = true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			params.Set("provider", pt2.Provider)
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ProviderTemplateID).To(Equal(pt2.ID))
		})

		It("uses a custom template if provided by admin", func() {
			_, err := db.Exec(`UPDATE users SET admin = true WHERE id = $1;`, u.ID)
			Expect(err).To(BeNil())

			stack, err := test.CreateHostStack(db, &test.HostStack{
				R:  &region.Region{Slug: "us-west-2"},
				P:  &plan.Plan{Slug: "huge", Price: 500},
				PT: &providertemplate.ProviderTemplate{Name: "usw2-huge-disabled", Disabled: true, Provider: "dummy"},
			})
			Expect(err).To(BeNil())

			params.Set("template_name", "usw2-huge-disabled")
			doRequest(params)

			hosts, _ := host.FindAllByUser(db, u.ID)
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ProviderTemplateID).To(Equal(stack.PT.ID))
			Expect(hosts[0].RegionID).To(Equal(stack.R.ID), "failed to override supplied region")
			Expect(hosts[0].RegionID).ToNot(Equal(hs.R.ID), "failed to override supplied region")
			Expect(hosts[0].PlanID).To(Equal(stack.P.ID), "failed to override supplied plan")
			Expect(hosts[0].PlanID).ToNot(Equal(hs.P.ID), "failed to override supplied plan")
		})

		It("allows a hidden plan", func() {
			planSlug := "1gb-always-on"
			_, err := test.CreateHostStack(db, &test.HostStack{
				P: &plan.Plan{Hidden: true, Slug: planSlug, Price: 500},
				R: hs.R,
			})
			Expect(err).To(BeNil())

			params.Set("size", planSlug)
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when hostbuilder fails to create the host", func() {
		BeforeEach(func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			replaceTransport(hb, &test.FakeRoundTripper{
				Message: errorResponse,
				Status:  http.StatusBadRequest,
			})

			hosts.HostbuilderClient = hb
		})

		It("creates a host in the orphan state", func() {
			doRequest(params)
			hosts := []host.Host{}
			err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL AND preprovisioned = FALSE ORDER BY id;", u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.Slug).To(HavePrefix(params.Get("name")))
			Expect(slugbuilder.IsValid(h.Slug)).To(BeTrue())
			Expect(h.State).To(Equal(host.StateOrphan))
		})

		It("should update host provider identifier and ip address if available regardless of error", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			replaceTransport(hb, &test.FakeRoundTripper{
				Message: `{"id": "i-67263","ip_address":"127.0.0.1","error":"some error occurred"}`,
				Status:  http.StatusBadRequest,
			})

			hosts.HostbuilderClient = hb

			doRequest(params)
			hosts := []host.Host{}
			err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL AND preprovisioned = FALSE ORDER BY id;", u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.Slug).To(HavePrefix(params.Get("name")))
			Expect(slugbuilder.IsValid(h.Slug)).To(BeTrue())
			Expect(h.State).To(Equal(host.StateOrphan))
			Expect(h.ProviderIdentifier).To(Equal(sql.NullString{"i-67263", true}))
			Expect(h.IPAddress).To(Equal(sql.NullString{"127.0.0.1", true}))
		})
	})

	Context("when async param is true", func() {
		BeforeEach(func() {
			params.Set("async", "true")
		})

		It("returns 202 accepted and creates host asynchronously", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Host.ID > 0).To(BeTrue(), "missing id. not saved ?")
			Expect(j.Host.State).To(Equal(host.StateBorn), "unexpected host state")

			// These fields are not yet set.
			Expect(j.Host.IPAddress).To(BeEmpty(), "unexpected ip address")
			Expect(j.Host.FreighterdPort).To(BeZero(), "unexpected freighterd port")
		})

		Context("when hostbuilder successfully creates a host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &test.FakeRoundTripper{
					Message: fmt.Sprintf(`{"id":"abcd1234","ip_address":"0.0.0.0","provider":"%s","freighterd_port":65333}`, hs.PT.Name),
					Status:  http.StatusOK,
				}
				replaceTransport(hb, rt)

				hosts.HostbuilderClient = hb
			})

			It("publishes host events with the created host", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventHostCreated")
				case <-finished:
					hosts, err := host.FindAllByUser(db, u.ID)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					h := hosts[0]

					Expect(msgs[0]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreating,
						Data: &controllers.ProgressEvent{
							Slug: h.Slug,
							Progress: fmt.Sprintf(
								`{"id":"abcd1234","ip_address":"0.0.0.0","provider":"%s","freighterd_port":65333}`, hs.PT.Name),
						},
					}), "unexpected event, wanted EventHostCreating")

					Expect(msgs[1]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreated,
						Data: map[string]*serializers.Host{
							"host": serializers.SerializeHost(h),
						},
					}), "unexpected event, wanted EventHostCreated")
				}
			})

			Context("when user is free tier", func() {
				BeforeEach(func() {
					_, err := db.Exec(`UPDATE billing_profiles SET state=$1, max_hours=10 WHERE user_id=$2;`,
						billingprofile.StateFree, u.ID)
					Expect(err).To(BeNil())
				})

				It("creates the host", func() {
					_, err := db.Exec(`UPDATE plans SET price=0;`)
					Expect(err).To(BeNil())

					params.Set("container_template_slug", "rails")
					params.Set("container_name", "my container")

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))
				})

				It("responds with error if paid plan is selected", func() {
					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusForbidden))

					errType, _ := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("paid_plan_not_allowed"))
				})

				It("responds with HTTP 403 Forbidden when user has created host before", func() {
					_, err := db.Exec(`UPDATE plans SET price=0;`)
					Expect(err).To(BeNil())

					_, err = test.CreateHostStack(db, &test.HostStack{U: u})
					Expect(err).To(BeNil())

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusForbidden))

					errType, _ := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("max_free_host_reached"))
				})

				It("responds with no error if user has orphaned terminated host", func() {
					_, err := db.Exec(`UPDATE plans SET price=0;`)
					Expect(err).To(BeNil())

					hs, err := test.CreateHostStack(db, &test.HostStack{U: u})
					Expect(err).To(BeNil())

					err = hs.H.Delete(db)
					Expect(err).To(BeNil())
					Expect(hs.H.TerminatedAt).ToNot(BeNil())

					_, err = db.Exec(`UPDATE hosts SET state = $1 WHERE id = $2`, host.StateOrphan, hs.H.ID)
					Expect(err).To(BeNil())

					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusAccepted))
				})

				It("responds with HTTP 403 Forbidden when selected region has exceeded daily quota", func() {
					origQuota := envvars.FreeHostQuota
					envvars.FreeHostQuota = "2"
					defer func() { envvars.FreeHostQuota = origQuota }()

					_, err := db.Exec(`UPDATE plans SET price=0;`)
					Expect(err).To(BeNil())

					freePlan := &plan.Plan{
						Slug:  "free-plan",
						Price: 0,
					}
					Expect(freePlan.Insert(db)).To(BeNil())

					hs1, err := test.CreateHostStack(db, &test.HostStack{P: freePlan})
					Expect(err).To(BeNil())

					h2 := &host.Host{
						Slug:               "my-host-2",
						State:              host.StateRunning,
						UserID:             hs1.U.ID,
						PlanID:             freePlan.ID,
						RegionID:           hs1.R.ID,
						ProviderTemplateID: hs1.PT.ID,
					}
					Expect(h2.Insert(db)).To(BeNil())

					h3 := &host.Host{
						Slug:               "my-host-3",
						State:              host.StateRunning,
						UserID:             hs1.U.ID,
						PlanID:             freePlan.ID,
						RegionID:           hs1.R.ID,
						ProviderTemplateID: hs1.PT.ID,
					}
					Expect(h3.Insert(db)).To(BeNil())

					params.Set("region", hs1.R.Slug)
					params.Set("size", freePlan.Slug)
					res := doRequest(params)
					Expect(res.Code).To(Equal(http.StatusForbidden))

					errType, _ := test.ErrorSlug(res.Body)
					Expect(errType).To(Equal("max_region_free_hosts_reached"))
				})
			})
		})

		Context("when hostbuilder fails to create the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: `{"error":"some error occurred"}`,
					Status:  http.StatusBadRequest,
				})

				hosts.HostbuilderClient = hb
			})

			It("updates the host's state to orphan", func() {
				doRequest(params)

				Eventually(func() (string, error) {
					hosts := []host.Host{}
					err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL AND preprovisioned = FALSE ORDER BY id;", u.ID)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					h := hosts[0]

					return h.State, err
				}).Should(Equal(host.StateOrphan))
			})

			It("publishes a host create failed event with the host record and error", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-finished:
					hosts := []host.Host{}
					err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL AND preprovisioned = FALSE ORDER BY id;", u.ID)
					Expect(err).To(BeNil())
					Expect(hosts).To(HaveLen(1))
					h := &hosts[0]

					Expect(msgs[0]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreating,
						Data: &controllers.ProgressEvent{Slug: h.Slug,
							Progress: `{"error":"some error occurred"}`},
					}))

					evt, ok := msgs[1].(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(evt.Name).To(Equal(controllers.EventHostCreateFailed))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(evtData["host"]).To(Equal(serializers.SerializeHost(h)))
					// Original error caused by hostbuilder error is masked.
					Expect(evtData["error"]).To(ContainSubstring("failed to create workspace"))
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventHostCreateFailed")
				}
			})
		})
	})

	Context("when creating host under org", func() {
		BeforeEach(func() {
			err := db.WithTransaction(func(t *sqlx.Tx) error {
				return billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})
			})
			Expect(err).To(BeNil())
			_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE plans SET price=123, org=true;`)
			Expect(err).To(BeNil())

			params.Set("org", o.Slug)
		})

		It("creates the host", func() {
			validStates := []string{
				billingprofile.StatePaid,
				billingprofile.StateExempt,
				billingprofile.StatePaymentDue,
				billingprofile.StateFree,
			}
			for _, st := range validStates {
				_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2`, st, o.ID)
				Expect(err).To(BeNil())

				params.Set("async", "true")
				res := doRequest(params)
				Expect(res.Code).To(Equal(http.StatusAccepted))
			}
		})

		It("responds with no credit card error when org does not have billing profile", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2`, billingprofile.StateNoCard, o.ID)
			Expect(err).To(BeNil())

			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("request_not_allowed"))
			Expect(errDesc).To(Equal("your organization has not added a credit card"))
		})

		It("responds with unpaid invoices error when org has delinquent billing profile", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2`, billingprofile.StateDelinquent, o.ID)
			Expect(err).To(BeNil())

			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("outstanding_invoices"))
			Expect(errDesc).To(Equal("your organization has outstanding invoices"))
		})

		It("responds with hosts limit reached when org has hit a limit ", func() {
			_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE org_id = $2`, billingprofile.StatePaid, o.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE orgs SET max_hosts = 1 WHERE id = $1;`, o.ID)
			Expect(err).To(BeNil())
			_, err = test.CreateHostStack(db, &test.HostStack{O: o})
			Expect(err).To(BeNil())

			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("max_hosts_reached"))
			Expect(errDesc).To(Equal("your organization has reached the maximum number of workspaces"))
		})

		It("does not allow orgs to create a free host", func() {
			bp, err := billingprofile.FindForOrg(db, o.ID)
			Expect(err).To(BeNil())
			bp.State = billingprofile.StateFree
			Expect(bp.Update(db)).To(BeNil())

			_, err = db.Exec(`UPDATE plans SET price=0, org=true;`)
			Expect(err).To(BeNil())

			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("free_plan_not_allowed"))
			Expect(errDesc).To(Equal("You are not allowed to create a free tier workspace"))
		})
	})

	Context("when creating host under multi host subscription", func() {
		var (
			mPl  *plan.Plan
			mSub *subscription.Subscription
		)
		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			mPl = &plan.Plan{
				Slug:          "multiPlan",
				Name:          "multiPlan",
				MaxContainers: 1,
				Price:         1499,
				StripePlanID:  sql.NullString{"multi-plan-id", true},
				MaxHosts:      sql.NullInt64{1, true},
			}
			Expect(mPl.Insert(db)).To(BeNil())

			tem := &providertemplate.ProviderTemplate{
				Name:     "default-2",
				Provider: hs.PT.Provider,
				RegionID: hs.PT.RegionID,
				PlanID:   mPl.ID,
			}
			Expect(tem.Insert(db)).To(BeNil())

			mSub = &subscription.Subscription{
				PlanID:           mPl.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"sub_1", true},
				Slug:             "SLUG",
				MultiHost:        true,
			}
			err = test.WrapInTransaction(db, mSub.Insert)
			Expect(err).To(BeNil())

			params.Del("size")

			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: fmt.Sprintf(`
				{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
				`, hs.PT.Name),
				Status: http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb
		})

		It("should prevent a misconfigured plan from being used", func() {
			_, err := db.Exec("UPDATE plans SET max_hosts = NULL WHERE id = $1", mPl.ID)
			Expect(err).To(BeNil())

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusInternalServerError))

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(BeEmpty())
		})

		It("should create a host assigning it to an existing subscription", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			host := hosts[0]
			Expect(host.SubscriptionID.Int64).To(Equal(mSub.ID))
			Expect(host.PlanID).To(Equal(mPl.ID))
		})

		It("should not allow another host if the plan's MaxHost has been exceeded", func() {
			h1 := &host.Host{
				UserID:             u.ID,
				Slug:               "multi-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				SubscriptionID:     sql.NullInt64{mSub.ID, true},
			}
			Expect(h1.Insert(db)).To(BeNil())

			h2 := &host.Host{
				UserID:             u.ID,
				Slug:               "multi-host-2",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				SubscriptionID:     sql.NullInt64{mSub.ID, true},
			}
			Expect(h2.Insert(db)).To(BeNil())

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusForbidden))

			errType, errDesc := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("max_hosts_reached"))
			Expect(errDesc).To(Equal("You have exceeded the number of hosts for this plan, please upgrade"))
		})

		It("should support orgs", func() {
			err := db.WithTransaction(func(t *sqlx.Tx) error {
				return billingprofile.Create(t, u.ID, sql.NullInt64{o.ID, true})
			})
			Expect(err).To(BeNil())
			_, err = orgmembership.AddMember(db, o.ID, strconv.FormatInt(u.ID, 10), orgmembership.RoleMember)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE plans SET org=true WHERE id = $1;`, mPl.ID)
			Expect(err).To(BeNil())

			params.Set("org", o.Slug)

			bp, err := billingprofile.FindForOrg(db, o.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`UPDATE billing_profiles SET state='paid' WHERE id = $1;`, bp.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE subscriptions SET billing_profile_id = $1 WHERE id = $2", bp.ID, mSub.ID)
			Expect(err).To(BeNil())

			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))

			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			host := hosts[0]
			Expect(host.SubscriptionID.Int64).To(Equal(mSub.ID))
			Expect(host.PlanID).To(Equal(mPl.ID))
			Expect(host.OrgID.Int64).To(Equal(o.ID))
		})
	})

	It("responds with no credit card error when user does not have billing profile", func() {
		_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2`, billingprofile.StateNoCard, u.ID)
		Expect(err).To(BeNil())

		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusForbidden))
		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("request_not_allowed"))
		Expect(errDesc).To(Equal("you have not added a credit card"))
	})

	It("responds with unpaid invoices error when user has delinquent billing profile", func() {
		_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2`, billingprofile.StateDelinquent, u.ID)
		Expect(err).To(BeNil())

		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))
		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("outstanding_invoices"))
		Expect(errDesc).To(Equal("you have outstanding invoices, please check your billing settings"))
	})

	It("responds with hosts limit reached when a user has hit a limit ", func() {
		_, err := db.Exec(`UPDATE user_profiles SET max_hosts = 1 WHERE user_id = $1;`, u.ID)
		Expect(err).To(BeNil())

		_, err = test.CreateHostStack(db, &test.HostStack{U: u})
		Expect(err).To(BeNil())

		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusForbidden))
		errType, errDesc := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("max_hosts_reached"))
		Expect(errDesc).To(Equal("you have reached the maximum number of workspaces"))
	})

	Context("oauth token failure modes", func() {
		BeforeEach(func() {
			authTokenInputs.Path = "/api/v0/hosts"
			authTokenInputs.Method = "POST"
		})

		test.ItRequiresAuthToken(&authTokenInputs)
	})
})
