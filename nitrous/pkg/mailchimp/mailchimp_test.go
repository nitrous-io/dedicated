package mailchimp_test

import (
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/mailchimp")
}

var _ = Describe("pkg/mailchimp", func() {
	var (
		server *ghttp.Server

		mc = &mailchimp.MailChimp{}
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		envvars.MailChimpAPIUrl = server.URL()
		envvars.MailChimpAPIKey = "apissvc4"
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("Subscribe()", func() {
		var (
			opts *mailchimp.SubscribeOptions
		)

		BeforeEach(func() {
			opts = &mailchimp.SubscribeOptions{
				Email: "mailme@callumj.com",
				MergeFields: map[string]interface{}{
					"FNAME": "Callum Jones",
				},
			}
		})

		It("should return an error if the listID isn't present", func() {
			err := mc.Subscribe("", opts)
			Expect(err).To(Equal(mailchimp.ErrNoListID))
		})

		It("should return an error if the APIUrl isn't present", func() {
			envvars.MailChimpAPIUrl = ""

			err := mc.Subscribe("sa", opts)
			Expect(err).To(Equal(mailchimp.ErrMailChimpAPIUrlNotSet))
		})

		It("should return an error if the APIKey isn't present", func() {
			envvars.MailChimpAPIKey = ""

			err := mc.Subscribe("sa", opts)
			Expect(err).To(Equal(mailchimp.ErrMailChimpAPIKeyNotSet))
		})

		It("should submit the JSON request", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/3.0/lists/sa/members"),
				ghttp.VerifyContentType("application/json"),
				ghttp.VerifyBasicAuth("ignored", envvars.MailChimpAPIKey),
				ghttp.VerifyJSON(`{
                                        "email_address": "mailme@callumj.com",
                                        "status": "subscribed",
                                        "merge_fields": {
                                                "FNAME": "Callum Jones"
                                        }
                                }`),
				ghttp.RespondWith(200, nil),
			))

			err := mc.Subscribe("sa", opts)
			Expect(err).To(BeNil())
		})

		It("should return an error", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/3.0/lists/sa/members"),
				ghttp.VerifyContentType("application/json"),
				ghttp.VerifyBasicAuth("ignored", envvars.MailChimpAPIKey),
				ghttp.RespondWith(400, "Sorry, we failed!"),
			))

			err := mc.Subscribe("sa", opts)
			Expect(err).To(MatchError(`Error performing "lists/sa/members", err: "Sorry, we failed!"`))
		})
	})

	Describe("Unsubscribe()", func() {
		It("should return an error if the listID isn't present", func() {
			err := mc.Unsubscribe("", "urist.mcvankab@freddiesjokes.com")
			Expect(err).To(Equal(mailchimp.ErrNoListID))
		})

		It("should return an error if the APIUrl isn't present", func() {
			envvars.MailChimpAPIUrl = ""

			err := mc.Unsubscribe("sa", "urist.mcvankab@freddiesjokes.com")
			Expect(err).To(Equal(mailchimp.ErrMailChimpAPIUrlNotSet))
		})

		It("should return an error if the APIKey isn't present", func() {
			envvars.MailChimpAPIKey = ""

			err := mc.Unsubscribe("sa", "urist.mcvankab@freddiesjokes.com")
			Expect(err).To(Equal(mailchimp.ErrMailChimpAPIKeyNotSet))
		})

		It("should submit the DELETE request", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("DELETE", "/3.0/lists/sa/members/62eeb292278cc15f5817cb78f7790b08"),
				ghttp.VerifyBasicAuth("ignored", envvars.MailChimpAPIKey),
				ghttp.RespondWith(200, nil),
			))

			err := mc.Unsubscribe("sa", "urist.mcvankab@freddiesjokes.com")
			Expect(err).To(BeNil())
		})

		It("should return an error", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("DELETE", "/3.0/lists/sa/members/62eeb292278cc15f5817cb78f7790b08"),
				ghttp.VerifyBasicAuth("ignored", envvars.MailChimpAPIKey),
				ghttp.RespondWith(400, "Sorry, we failed!"),
			))

			err := mc.Unsubscribe("sa", "urist.mcvankab@freddiesjokes.com")
			Expect(err).To(MatchError(`Error performing "lists/sa/members/62eeb292278cc15f5817cb78f7790b08", err: "Sorry, we failed!"`))
		})
	})
})
