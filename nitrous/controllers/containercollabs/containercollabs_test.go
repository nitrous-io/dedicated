package containercollabs_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containercollabs"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	lgr "github.com/nitrous-io/nitrous-common/logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containercollabs")
}

var _ = Describe("controllers/containercollabs", func() {
	var (
		db  *db.Database
		u   *user.User
		t   *token.Token
		cnr *container.Container

		logger  *lgr.Logger
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		logger = &lgr.Logger{Level: lgr.LogLevelQuiet}
		svr = server.NewServer("", db, logger, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()

		u = &user.User{
			Username: "owner",
			Email:    "owner@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		hs, err := test.CreateHostStack(db, &test.HostStack{
			U: u,
		})
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
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
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /containers/{slug}/collaborators", func() {
		doRequest := func() {
			req, err := test.NewRequest("GET", fmt.Sprintf("/api/v0/containers/%s/collaborators", cnr.Slug), nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)

			handler.ServeHTTP(res, req)
		}

		It("should return an empty response when nothing is shared", func() {
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				ContainerCollabs []*serializers.ContainerCollabUser `json:"container_collabs"`
			}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.ContainerCollabs).To(BeEmpty())
		})

		Context("with collaborators", func() {
			var all []*containercollab.ContainerCollabUser
			var u1, u2 *user.User

			BeforeEach(func() {
				u1 = &user.User{
					Username: "foo",
					Email:    "foo@example.com",
					Password: "123456",
				}
				err := test.WrapInTransaction(db, u1.Insert)
				Expect(err).To(BeNil())

				u2 = &user.User{
					Username: "bar",
					Email:    "bar@example.com",
					Password: "123456",
				}
				err = test.WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u1.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2) RETURNING ID`, cnr.ID, u2.ID)
				Expect(err).To(BeNil())

				all, err = containercollab.FindAll(db, cnr.ID)
				Expect(err).To(BeNil())
			})

			It("should return all container collaborators for a container", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					ContainerCollabs []*serializers.ContainerCollabUser `json:"collaborators"`
				}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.ContainerCollabs).To(HaveLen(2))
				Expect([]int64{j.ContainerCollabs[0].ContainerID, j.ContainerCollabs[1].ContainerID}).To(Equal([]int64{all[0].ContainerID, all[1].ContainerID}))
			})

			It("should not include deleted users", func() {
				_, err := db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1`, u2.ID)
				Expect(err).To(BeNil())

				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					ContainerCollabs []*serializers.ContainerCollabUser `json:"collaborators"`
				}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j.ContainerCollabs).To(HaveLen(1))
				Expect(j.ContainerCollabs[0].ContainerID).To(Equal(all[0].ContainerID))
			})

		})
	})

	Describe("POST /containers/{slug}/collaborators", func() {
		var (
			fakeMailer *test.FakeMailer
			origMailer mailer.Mailer
		)

		BeforeEach(func() {
			envvars.WebHostname = "nitrous.example"
			origMailer = user.Mailer
			fakeMailer = &test.FakeMailer{}
			containercollabs.Mailer = fakeMailer
		})

		AfterEach(func() {
			user.Mailer = origMailer
		})

		doRequest := func(usr string) {
			params := &url.Values{}
			params.Add("user", usr)
			req, err := test.NewFormRequest("POST", fmt.Sprintf("/api/v0/containers/%s/collaborators", cnr.Slug), params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)

			handler.ServeHTTP(res, req)
		}

		It("should return an error if no user is provided", func() {
			doRequest("")

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("`user` must be provided"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the user could not be located", func() {
			doRequest("donotfindme")

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("user_not_found"))
			Expect(errDesc).To(Equal("the specified user could not be found"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the specified user is the container owner", func() {
			doRequest(u.Username)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("same_user"))
			Expect(errDesc).To(Equal("you cannot add yourself as a collaborator"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the user has been deleted", func() {
			otherUser := &user.User{
				Username: "serviceaccount",
				Email:    "service@nitrous.io",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1;`, otherUser.ID)
			Expect(err).To(BeNil())

			doRequest("serviceaccount")

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("user_not_found"))
			Expect(errDesc).To(Equal("the specified user could not be found"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should return an error if the user is a service account", func() {
			otherUser := &user.User{
				Username: "serviceaccount",
				Email:    "service@nitrous.io",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE users SET service_account = TRUE WHERE id = $1;`, otherUser.ID)
			Expect(err).To(BeNil())

			doRequest("serviceaccount")

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("user_not_found"))
			Expect(errDesc).To(Equal("the specified user could not be found"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})

		It("should add the user as a collaborator", func() {
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			doRequest("steve")

			Expect(res.Code).To(Equal(http.StatusOK))
			var j struct {
				Success bool `json:"success"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.Success).To(BeTrue())

			Eventually(func() interface{} {
				collab := &containercollab.ContainerCollab{}
				err = db.Get(collab, "SELECT * FROM container_collabs WHERE user_id = $1 AND container_id = $2", otherUser.ID, cnr.ID)
				Expect(err).To(BeNil())
				Expect(collab).ToNot(BeNil())

				return collab.EmailSentAt.Valid
			}).Should(BeTrue())

			Expect(fakeMailer.From).To(Equal(`"Nitrous Pro" <mailer@nitrous.io>`))
			Expect(fakeMailer.Tos).To(Equal([]string{`"steve" <steve@stevey.com>`}))
			Expect(fakeMailer.ReplyTo).To(Equal(`"Nitrous Pro" <mailer@nitrous.io>`))
			Expect(fakeMailer.Subject).To(Equal("owner has shared container java-test-container with you!"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hello steve!"))
			Expect(fakeMailer.Body).To(ContainSubstring("https://nitrous.example/app/#/containers/java-test-container"))
			Expect(fakeMailer.HTML).To(ContainSubstring("<h3>Hello steve!</h3>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/containers/java-test-container" target="_blank">`))
		})

		It("should add the user as a collaborator even if the mailer fails", func() {
			fakeMailer.Error = errors.New("Sorry mate!")
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			doRequest("steve")

			Expect(res.Code).To(Equal(http.StatusOK))
			var j struct {
				Success bool `json:"success"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.Success).To(BeTrue())

			Eventually(func() interface{} {
				return fakeMailer.SendMailCalled
			}).Should(BeTrue())

			collab := &containercollab.ContainerCollab{}
			err = db.Get(collab, "SELECT * FROM container_collabs WHERE user_id = $1 AND container_id = $2", otherUser.ID, cnr.ID)
			Expect(err).To(BeNil())
			Expect(collab).ToNot(BeNil())

			Expect(collab.EmailSentAt.Valid).To(BeFalse())
		})

		It("should fire off events to the target user and container owner", func() {
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			chOwner := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(chOwner, u.Topic())
			chCollab := svr.Events.Subscribe(otherUser.Topic())
			defer svr.Events.Unsubscribe(chCollab, otherUser.Topic())

			doRequest("steve")

			msgsOwners := []pubsub.Message{}
			msgsCollab := []pubsub.Message{}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				msgsOwners = append(msgsOwners, <-chOwner)
				wg.Done()
			}()
			go func() {
				msgsCollab = append(msgsCollab, <-chCollab)
				wg.Done()
			}()

			wg.Wait()
			Expect(msgsOwners).To(HaveLen(1))
			Expect(msgsCollab).To(HaveLen(1))

			ownerMsg := msgsOwners[0].(*controllers.Event)
			Expect(ownerMsg.Name).To(Equal(controllers.EventCollaboratorAdded))
			data, ok := ownerMsg.Data.(map[string]interface{})
			Expect(ok).To(BeTrue())
			cn, ok := data["container"].(*serializers.Container)
			Expect(cn.ID).To(Equal(cnr.ID))

			ownerCollab := msgsCollab[0].(*controllers.Event)
			Expect(ownerCollab.Name).To(Equal(controllers.EventCollaboratorAdded))
			data, ok = ownerCollab.Data.(map[string]interface{})
			Expect(ok).To(BeTrue())
			cn, ok = data["container"].(*serializers.Container)
			Expect(cn.ID).To(Equal(cnr.ID))
		})

		It("should not allow the same user to be added twice", func() {
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())

			_, err = containercollab.Create(db, cnr.ID, otherUser.ID)
			Expect(err).To(BeNil())

			doRequest("steve")

			Expect(res.Code).To(Equal(http.StatusConflict))

			errSlug, errDesc := test.ErrorSlug(res.Body)
			Expect(errSlug).To(Equal("already_added"))
			Expect(errDesc).To(Equal("this user is already a collaborator"))

			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})
	})

	Describe("DELETE /containers/{slug}/collaborators/{ccid}", func() {
		doRequest := func(ccid int64) {
			req, err := test.NewRequest("DELETE", fmt.Sprintf("/api/v0/containers/%s/collaborators/%d", cnr.Slug, ccid), nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)

			handler.ServeHTTP(res, req)
		}

		It("should perform a delete", func() {
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())
			cc, err := containercollab.Create(db, cnr.ID, otherUser.ID)
			Expect(err).To(BeNil())

			doRequest(cc.ID)

			Expect(res.Code).To(Equal(http.StatusOK))
			var j struct {
				Success bool `json:"success"`
			}
			err = json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			Expect(j.Success).To(BeTrue())

			var count int
			err = db.QueryRow(`SELECT COUNT(*) FROM container_collabs WHERE id = $1;`, cc.ID).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("should fire off events to the target user and container owner", func() {
			otherUser := &user.User{
				Username: "steve",
				Email:    "steve@stevey.com",
				Password: "Password1",
			}
			err := test.WrapInTransaction(db, otherUser.Insert)
			Expect(err).To(BeNil())
			cc, err := containercollab.Create(db, cnr.ID, otherUser.ID)
			Expect(err).To(BeNil())

			chOwner := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(chOwner, u.Topic())
			chCollab := svr.Events.Subscribe(otherUser.Topic())
			defer svr.Events.Unsubscribe(chCollab, otherUser.Topic())

			doRequest(cc.ID)

			msgsOwners := []pubsub.Message{}
			msgsCollab := []pubsub.Message{}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				msgsOwners = append(msgsOwners, <-chOwner)
				wg.Done()
			}()
			go func() {
				msgsCollab = append(msgsCollab, <-chCollab)
				wg.Done()
			}()

			wg.Wait()
			Expect(msgsOwners).To(HaveLen(1))
			Expect(msgsCollab).To(HaveLen(1))

			ownerMsg := msgsOwners[0].(*controllers.Event)
			Expect(ownerMsg.Name).To(Equal(controllers.EventCollaboratorRemoved))
			data, ok := ownerMsg.Data.(map[string]interface{})
			Expect(ok).To(BeTrue())
			cn, ok := data["container"].(*serializers.Container)
			Expect(cn.ID).To(Equal(cnr.ID))

			ownerCollab := msgsCollab[0].(*controllers.Event)
			Expect(ownerCollab.Name).To(Equal(controllers.EventCollaboratorRemoved))
			data, ok = ownerCollab.Data.(map[string]interface{})
			Expect(ok).To(BeTrue())
			cn, ok = data["container"].(*serializers.Container)
			Expect(cn.ID).To(Equal(cnr.ID))
		})
	})

})
