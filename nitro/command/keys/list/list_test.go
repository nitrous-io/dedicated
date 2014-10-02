package list_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/keys/list"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "keys/list")
}

var _ = Describe("keys/list", func() {
	var (
		cmd          *list.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string

		responseJSON = strings.TrimSpace(`
{"keys":[{"id":6,"container_id":"cc41ff1","fingerprint":"b4:a0:8b:85:b0:b1:c3:d5:7b:7a:85:e1:01:a8:a7:ad","key":"ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAwJiXZ7y2PnwujFplRKJfLa2JH2EXgp2lkKFMBDhH36z5N16CheJYGTqxVNQTR1vfxzBypl8+s8/RngcSHL7AW/si+zP6iOAEpAaHC0WDPY2YtDZ0ikhw2zwrfOdw0iMny54ouihYA9kkei3tf2sYbbwRHP5qxfjtGEdzTqL6JTwPhRQ/j6DW0ieH7LOebr/sAJ4RYaFgobhPe6+ktNDK83hSeBRxxsYzRNdASEdjFhMq1ia68eGESMtCiwgoAlP8K9zyLKzBbiYfU1S90AonulEZqOiYUtYwlm5Y5UoKqF6E2D+u9JTS+cIIU+/KsSzKLxvMUJEpulWQw5XNTxcxiw==\n"},{"id":2,"container_id":"cc41ff1","fingerprint":"6b:26:9a:7b:91:17:04:c3:4f:29:29:af:ac:e9:51:06","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDeY3XzBA6nK0FBR6L4xYkVqxCrt5oSF3Zipf95hKuq5zB0Q3RE+8mHorkDInu2Lc+fyOggE7ZUxrTuGjAk8Y+DIfgva3jdJRCDqsp/Q1UeUMyv6ZnG+sLjq5lvmcDprRsqfPLX13o1NUuw3DrJDAeiwXHJwk/MctBDeiwy4FClXCu55JhSmOOv1iPZDLNBZqwGJcMVe2amfWmprtSXc2pnVBU51XJBuEnaC8To+YC3vs46g8wTXzeIQHlpJsgVvdA2OxnYhzsAXJKgF595j+7uwqQP1+BBNhwNSbwI8NTuWBFcIkFBpSl4OnhUhybXKMUiecuq0/plfvXwT9aD423h\n"},{"id":1,"container_id":"cc41ff1","fingerprint":"dd:e7:f9:3c:84:5a:65:84:bf:ea:4a:2c:3d:60:e4:f3","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDQyswKwwabS0fyKQISyVY9DQvMXpEaifzqZhtGPXquOLGrMN6GSK27LTUJ+J9U8EWooKu2NWlFPhvKcB2EknVKnra8RNjm77xmf5CkZZzyc5O0CYi8mIzsprm16TcJD6cNkp2zDeuUnyAf9at2hwGpRGkUvDtJ9bPQhvyYn5hN1Rl8zHvTGw5jc1Pw1RxEFvkKQ/P0yH3eOmJoGu/iHklzRRVpqD38A6YnuqnERcpUEYC/++8H3RnFg5FNDTqnEyJTgBFYxoNm7vfHk7g/FywZAzbwNdEq8QsvUqgpjODB44O2HA0QxZ/4+pBNLxP2iK58AjqZhXfg3OG76sHZd+of\n"},{"id":3,"container_id":"cc42ff1","fingerprint":"29:86:02:ee:61:91:5a:09:3d:2f:69:39:1b:93:73:ee","key":"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDAOUju2lvnGAVplM2ISo1AhoESQCeglOJCAJRoMj5CXmIub4Es0lQHRmmWmaR/xK/eLZ1gRbtc2TLNZr8TXFSR+oDA6zYZk7OiwFcN2tiaEjmX4pn6I/4k/Rz7zHnlMw1Vn3TOtFUpMmKzxEQc/BPU6jtiSiuPteGcgMkD/9UP80uyHe6SElFKy3tlNS4L9LwzQOEzqWU0rYn4Sqe/Y+RpOGut7Fqi/NLHv24ChhatTU/zmv4My8D1CVQFmzN4DfTwmosLNUt7jFdwyKcPbQp0l0y2hn93ZQh8mJPP9ohbYz/s7Ijx6E14cEveQaTYrpJlxvBAMZnkCsUGhcuFmY7H\n"}],"status":"ok"}
	`)

		responseOutput = strings.TrimSpace(`
Fingerprint                                      Key
b4:a0:8b:85:b0:b1:c3:d5:7b:7a:85:e1:01:a8:a7:ad  ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAwJiXZ7y2PnwujFplRKJfLa2JH2EXgp2lkKFMBDhH36z5N16CheJYGTqxVNQTR1vfxzBypl8+s8/RngcSHL7AW/si+zP6iOAEpAaHC0WDPY2YtDZ0ikhw2zwrfOdw0iMny54ouihYA9kkei3tf2sYbbwRHP5qxfjtGEdzTqL6JTwPhRQ/j6DW0ieH7LOebr/sAJ4RYaFgobhPe6+ktNDK83hSeBRxxsYzRNdASEdjFhMq1ia68eGESMtCiwgoAlP8K9zyLKzBbiYfU1S90AonulEZqOiYUtYwlm5Y5UoKqF6E2D+u9JTS+cIIU+/KsSzKLxvMUJEpulWQw5XNTxcxiw==
6b:26:9a:7b:91:17:04:c3:4f:29:29:af:ac:e9:51:06  ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDeY3XzBA6nK0FBR6L4xYkVqxCrt5oSF3Zipf95hKuq5zB0Q3RE+8mHorkDInu2Lc+fyOggE7ZUxrTuGjAk8Y+DIfgva3jdJRCDqsp/Q1UeUMyv6ZnG+sLjq5lvmcDprRsqfPLX13o1NUuw3DrJDAeiwXHJwk/MctBDeiwy4FClXCu55JhSmOOv1iPZDLNBZqwGJcMVe2amfWmprtSXc2pnVBU51XJBuEnaC8To+YC3vs46g8wTXzeIQHlpJsgVvdA2OxnYhzsAXJKgF595j+7uwqQP1+BBNhwNSbwI8NTuWBFcIkFBpSl4OnhUhybXKMUiecuq0/plfvXwT9aD423h
dd:e7:f9:3c:84:5a:65:84:bf:ea:4a:2c:3d:60:e4:f3  ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDQyswKwwabS0fyKQISyVY9DQvMXpEaifzqZhtGPXquOLGrMN6GSK27LTUJ+J9U8EWooKu2NWlFPhvKcB2EknVKnra8RNjm77xmf5CkZZzyc5O0CYi8mIzsprm16TcJD6cNkp2zDeuUnyAf9at2hwGpRGkUvDtJ9bPQhvyYn5hN1Rl8zHvTGw5jc1Pw1RxEFvkKQ/P0yH3eOmJoGu/iHklzRRVpqD38A6YnuqnERcpUEYC/++8H3RnFg5FNDTqnEyJTgBFYxoNm7vfHk7g/FywZAzbwNdEq8QsvUqgpjODB44O2HA0QxZ/4+pBNLxP2iK58AjqZhXfg3OG76sHZd+of
29:86:02:ee:61:91:5a:09:3d:2f:69:39:1b:93:73:ee  ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDAOUju2lvnGAVplM2ISo1AhoESQCeglOJCAJRoMj5CXmIub4Es0lQHRmmWmaR/xK/eLZ1gRbtc2TLNZr8TXFSR+oDA6zYZk7OiwFcN2tiaEjmX4pn6I/4k/Rz7zHnlMw1Vn3TOtFUpMmKzxEQc/BPU6jtiSiuPteGcgMkD/9UP80uyHe6SElFKy3tlNS4L9LwzQOEzqWU0rYn4Sqe/Y+RpOGut7Fqi/NLHv24ChhatTU/zmv4My8D1CVQFmzN4DfTwmosLNUt7jFdwyKcPbQp0l0y2hn93ZQh8mJPP9ohbYz/s7Ijx6E14cEveQaTYrpJlxvBAMZnkCsUGhcuFmY7H
	`) + "\n\n"

		container = "gregarious-scribe-24"
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/v0/containers/%s/keys", container)),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &list.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		// Reset it after every test so that any error conditions that
		// are set in contexts don't linger
		statusCode = http.StatusOK
		responseBody = `{"keys":[],"status":"ok"}`

		srv.Close()
	})

	doCmd := func() int {
		return cmd.Run([]string{container})
	}

	Context("successful API call which returns keys", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = responseJSON
		})

		It("outputs formatted keys on success", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(Equal(responseOutput))
			Expect(code).To(Equal(0))
		})
	})

	Context("Successful API call which returns no keys", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"keys":[],"status":"ok"}`
		})

		It("displays no keys found", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(Equal("No keys found\n"))
			Expect(code).To(Equal(0))
		})
	})

	Context("Unsuccessful API call which returns a JSON error message", func() {
		BeforeEach(func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"error": "validation_failed", "error_description": "container not found"}`
		})

		It("displays json error message", func() {
			code := doCmd()
			Expect(ui.ErrorWriter.String()).To(Equal("container not found\n"))
			Expect(code).To(Equal(1))
		})
	})
})
