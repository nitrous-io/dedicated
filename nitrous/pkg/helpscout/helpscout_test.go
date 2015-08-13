package helpscout_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/helpscout")
}

var _ = Describe("pkg/helpscout", func() {
	var (
		server *ghttp.Server

		statusCode   int
		responseBody string

		HelpScout = &helpscout.Conversation{}

		email    = "cats@example.com"
		username = "hellokitty"
		subject  = "hungrygames"
		name     = "cats"
		body     = "kitkat"
		metadata = "katniss"
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		helpscout.BaseURL = server.URL()
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("CreateConversation()", func() {
		BeforeEach(func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/conversations.json"),
				ghttp.VerifyContentType("application/json"),
				ghttp.VerifyBasicAuth(envvars.HelpScoutAPIKey, "ignored"),
				ghttp.RespondWithPtr(&statusCode, &responseBody),
			))
		})

		It("makes an HTTP POST request to the HelpScout create conversations API", func() {
			statusCode = http.StatusCreated
			responseBody = ""

			customer := helpscout.Person{
				Email:     email,
				Username:  username,
				FirstName: name,
				Type:      "customer",
			}
			mailbox := helpscout.MailboxRef{
				ID:   int(envvars.HelpScoutMailboxID),
				Name: envvars.HelpScoutMailboxName,
			}
			threads := []*helpscout.Thread{
				&helpscout.Thread{
					Type: "customer",
					CreatedBy: helpscout.Person{
						Email:     email,
						Username:  username,
						FirstName: name,
						Type:      "customer",
					},
					Body: body,
				},
				&helpscout.Thread{
					Type: "note",
					CreatedBy: helpscout.Person{
						ID:   envvars.HelpScoutUserID,
						Type: "user",
					},
					Body: metadata,
				},
			}

			err := HelpScout.CreateConversation(customer, subject, mailbox, threads)
			Expect(err).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("returns error with invalid mailbox params", func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"code":400,"error":"Input could not be validated","validationErrors":[{"property":"mailbox","value":null,"message":"Mailbox is required"}]}`

			customer := helpscout.Person{
				Email:     email,
				Username:  username,
				FirstName: name,
				Type:      "customer",
			}
			// Invalid Param: Missing Mailbox ID or Name
			mailbox := helpscout.MailboxRef{}
			threads := []*helpscout.Thread{
				&helpscout.Thread{
					Type: "customer",
					CreatedBy: helpscout.Person{
						Email:     email,
						Username:  username,
						FirstName: name,
						Type:      "customer",
					},
					Body: body,
				},
				&helpscout.Thread{
					Type: "note",
					CreatedBy: helpscout.Person{
						ID:   int64(envvars.HelpScoutUserID),
						Type: "user",
					},
					Body: metadata,
				},
			}

			expectedErrResponse := fmt.Errorf("CreateConversation: Unable to create conversation: expected %v, got %v - %s", http.StatusCreated, http.StatusBadRequest, responseBody)

			err := HelpScout.CreateConversation(customer, subject, mailbox, threads)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(expectedErrResponse))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("returns error with invalid customer params", func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"code":400,"error":"Input could not be validated","validationErrors":[{"property":"customer","value":null,"message":"Customer id or email is required"]}`

			customer := helpscout.Person{
				// Invalid Param: Missing Required email
				Username:  username,
				FirstName: name,
				Type:      "customer",
			}
			mailbox := helpscout.MailboxRef{
				ID:   int(envvars.HelpScoutMailboxID),
				Name: envvars.HelpScoutMailboxName,
			}
			threads := []*helpscout.Thread{
				&helpscout.Thread{
					Type: "customer",
					CreatedBy: helpscout.Person{
						Email:     email,
						Username:  username,
						FirstName: name,
						Type:      "customer",
					},
					Body: body,
				},
				&helpscout.Thread{
					Type: "note",
					CreatedBy: helpscout.Person{
						ID:   envvars.HelpScoutUserID,
						Type: "user",
					},
					Body: metadata,
				},
			}

			expectedErrResponse := fmt.Errorf("CreateConversation: Unable to create conversation: expected %v, got %v - %s", http.StatusCreated, http.StatusBadRequest, responseBody)

			err := HelpScout.CreateConversation(customer, subject, mailbox, threads)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(expectedErrResponse))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("returns error with invalid thread params", func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"code":400,"error":"Input could not be validated","validationErrors":[{"property":"threads","value":null,"message":"At least one thread is required"}]`

			customer := helpscout.Person{
				Email:     email,
				Username:  username,
				FirstName: name,
				Type:      "customer",
			}
			mailbox := helpscout.MailboxRef{
				ID:   int(envvars.HelpScoutMailboxID),
				Name: envvars.HelpScoutMailboxName,
			}
			// Invalid Param: Empty Thread
			threads := []*helpscout.Thread{}

			expectedErrResponse := fmt.Errorf("CreateConversation: Unable to create conversation: expected %v, got %v - %s", http.StatusCreated, http.StatusBadRequest, responseBody)

			err := HelpScout.CreateConversation(customer, subject, mailbox, threads)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(expectedErrResponse))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})
	})
})
