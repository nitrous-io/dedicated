package bonuses_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"

	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/bonuses")
}

var _ = Describe("controllers/bonuses", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		authTokenInputs test.AuthTokenInputs
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		t               *token.Token
		u               *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = NewTestServer("", db, nil)
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

	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /bonuses", func() {
		var (
			threeHours = int64(3 * 60 * 60) // in seconds
			fiveHours  = int64(5 * 60 * 60)
		)

		BeforeEach(func() {
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
		})

		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "GET", "/api/v0/bonuses", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("should return the bonuses if they exist", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Bonuses []*serializers.Bonus `json:"bonuses"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.Bonuses).To(HaveLen(4))

			slugs := []string{}
			amounts := []int64{}
			for i := range j.Bonuses {
				slugs = append(slugs, j.Bonuses[i].Slug)
				amounts = append(amounts, j.Bonuses[i].Amount)
			}

			Expect(slugs).To(ConsistOf("follow-twitter", "tweet", "like-facebook", "share-facebook"))
			Expect(amounts).To(ConsistOf(threeHours, fiveHours, threeHours, fiveHours))
		})
	})

	Describe("GET /api/v0/user/earned_bonuses", func() {
		doRequest := func() {
			var err error
			req, err = test.NewAuthedFormRequest(t, "GET", "/api/v0/user/earned_bonuses", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("serializes earned bonuses belonging to user", func() {
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
				{"follow-twitter", int64(3 * time.Hour)},
				{"tweet", int64(5 * time.Hour)},
				{"like-facebook", int64(3 * time.Hour)},
				{"share-facebook", int64(5 * time.Hour)},
			}

			for _, bonus := range bonuses {
				_, err := db.Exec("INSERT INTO bonuses (slug, amount) VALUES ($1, $2);", bonus.slug, bonus.amount)
				Expect(err).To(BeNil())
			}

			bn, err := bonus.FindBySlug(db, "follow-twitter")
			Expect(err).To(BeNil())

			ebn := &bonus.EarnedBonus{
				BonusID: bn.ID,
				UserID:  u.ID,
			}
			err = ebn.Insert(db)
			Expect(err).To(BeNil())

			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				EarnedBonuses []*serializers.EarnedBonus `json:"earned_bonuses"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())

			serializedEarnedBonuses := j.EarnedBonuses

			Expect(serializedEarnedBonuses).To(HaveLen(1))
			Expect(serializedEarnedBonuses[0].UserID).To(Equal(u.ID))
			Expect(serializedEarnedBonuses[0].BonusID).To(Equal(bn.ID))
		})
	})
})
