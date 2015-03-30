package inspect_test

import (
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/volumes/inspect"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "volumes/inspect")
}

var _ = Describe("volumes/inspect", func() {
	var (
		cmd          *inspect.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string

		responseJSON = `{"status":"ok","volume":{"slug":"dotfiles","size":123456789}}`
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/myhost/volumes/dotfiles/info"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &inspect.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	doCmd := func(human bool) int {
		args := []string{"myhost", "dotfiles"}
		if human {
			args = append(args, "--human-readable")
		}
		return cmd.Run(args)
	}

	Context("Successful API call", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = responseJSON
		})

		It("displays size", func() {
			code := doCmd(false)
			Expect(ui.OutputWriter.String()).To(ContainSubstring(`
Slug      Size (bytes)
dotfiles  123456789
`))
			Expect(code).To(Equal(0))
		})

		It("displays human-readable size", func() {
			code := doCmd(true)
			Expect(ui.OutputWriter.String()).To(ContainSubstring(`
Slug      Size
dotfiles  117.7M
`))
			Expect(code).To(Equal(0))
		})
	})

	Context("Unsuccessful API call which returns a JSON error message", func() {
		BeforeEach(func() {
			statusCode = http.StatusNotFound
			responseBody = `{"error": "not_found", "error_description": "volume could not be found"}`
		})

		It("displays json error message", func() {
			code := doCmd(false)
			Expect(ui.ErrorWriter.String()).To(Equal("volume could not be found\n"))
			Expect(code).To(Equal(1))
		})
	})

	Describe("humanReadable()", func() {
		cases := []struct {
			size   float64
			output string
		}{
			{0, "0"},
			{123, "123"},
			{1023, "1023"},
			{1024, "1.0K"},
			{math.Pow(1024, 2) - 1, "1024.0K"},
			{math.Pow(1024, 2), "1.0M"},
			{math.Pow(1024, 3) - 1, "1024.0M"},
			{math.Pow(1024, 3), "1.0G"},
			{math.Pow(1024, 4) - 1, "1024.0G"},
			{math.Pow(1024, 4), "1.0T"},
			{math.Pow(1024, 5) - 1, "1024.0T"},
			{math.Pow(1024, 5), "1024.0T"},
		}

		for _, c2 := range cases {
			c := c2
			size := uint64(c.size)
			It(fmt.Sprintf("converts %g to %s", c.size, c.output), func() {
				Expect(inspect.HumanReadable(size)).To(Equal(c.output))
				GinkgoRecover()
			})
		}
	})
})
