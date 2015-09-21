package hosts

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/models/user"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("buildCreateOptions", func() {
	It("accepts json request", func() {
		r, _ := http.NewRequest("POST", "/hosts", bytes.NewBufferString(`
		{
			"host": {
				"size": "1gb",
				"region": "us-west-2",
				"slug": "ultimate",
				"name": "windows XP",
				"async": true
			}
		}
		`))
		r.Header.Set("Content-Type", "application/json")

		opts, _ := buildCreateOptions(r, &user.User{})

		Expect(opts.Size).To(Equal("1gb"))
		Expect(opts.Region).To(Equal("us-west-2"))
		Expect(opts.Provider).To(Equal(defaultProvider))
		Expect(opts.Name).To(Equal("windows XP"))
		Expect(opts.Async).To(BeTrue())

		// slug is overridden
		Expect(opts.Slug).ToNot(Equal("ultimate"))
	})

	It("accepts form request", func() {
		data := url.Values{}
		data.Set("size", "1gb")
		data.Set("region", "us-west-2")
		data.Set("name", "windows XP")
		data.Set("async", "true")
		data.Set("slug", "ultimate")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		opts, _ := buildCreateOptions(r, &user.User{})

		Expect(opts.Size).To(Equal("1gb"))
		Expect(opts.Region).To(Equal("us-west-2"))
		Expect(opts.Provider).To(Equal(defaultProvider))
		Expect(opts.Name).To(Equal("windows XP"))
		Expect(opts.Async).To(BeTrue())

		// slug is overridden
		Expect(opts.Slug).ToNot(Equal("ultimate"))
	})

	It("rejects bypass provision when user is not admin", func() {
		data := url.Values{}
		data.Set("size", "1gb")
		data.Set("region", "us-west-2")
		data.Set("name", "windows XP")
		data.Set("bypass_preprovision", "true")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		opts, _ := buildCreateOptions(r, &user.User{})
		Expect(opts.BypassPreprovision).To(BeFalse())

		opts, _ = buildCreateOptions(r, &user.User{Admin: true})
		Expect(opts.BypassPreprovision).To(BeTrue())
	})

	It("rejects custom provider when user is not admin", func() {
		data := url.Values{}
		data.Set("size", "1gb")
		data.Set("region", "us-west-2")
		data.Set("name", "windows XP")
		data.Set("provider", "clown-compute")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		opts, _ := buildCreateOptions(r, &user.User{})
		Expect(opts.Provider).To(Equal(defaultProvider))

		opts, _ = buildCreateOptions(r, &user.User{Admin: true})
		Expect(opts.Provider).To(Equal("clown-compute"))
	})

	It("rejects custom provider template name when user is not admin", func() {
		data := url.Values{}
		data.Set("size", "1gb")
		data.Set("region", "us-west-2")
		data.Set("name", "windows XP")
		data.Set("template_name", "azure-sfo-1000GB")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		opts, _ := buildCreateOptions(r, &user.User{})
		Expect(opts.TemplateName).To(BeEmpty())

		opts, _ = buildCreateOptions(r, &user.User{Admin: true})
		Expect(opts.TemplateName).To(Equal("azure-sfo-1000GB"))
	})

	It("rejects empty size or region or name", func() {
		// empty size
		data := url.Values{}
		data.Set("region", "us-west-2")
		data.Set("name", "windows XP")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		_, err := buildCreateOptions(r, &user.User{})
		Expect(err).To(Equal(errSizeRequired))

		// empty region
		data = url.Values{}
		data.Set("size", "1gb")
		data.Set("name", "windows XP")
		r, _ = http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		_, err = buildCreateOptions(r, &user.User{})
		Expect(err).To(Equal(errRegionRequired))

		// empty name
		data = url.Values{}
		data.Set("size", "1gb")
		data.Set("region", "us-west-2")
		r, _ = http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		_, err = buildCreateOptions(r, &user.User{})
		Expect(err).To(Equal(errNameRequired))
	})

	It("generates slug with username if slugbuilder returns numeric result", func() {
		data := url.Values{}
		data.Set("region", "us-west-2")
		data.Set("name", "外人")
		data.Set("size", "1gb")
		r, _ := http.NewRequest("POST", "/hosts", strings.NewReader(data.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		opts, err := buildCreateOptions(r, &user.User{Username: "alien"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Slug).To(MatchRegexp(`\Aalien-\d+\z`))
	})
})
