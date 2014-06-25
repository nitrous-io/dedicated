package logintest

import (
	"fmt"

	"github.com/mitchellh/cli"
	. "github.com/nitrous-io/nitro/command/login"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoginCommand", func() {
	var (
		cmd *LoginCommand
		ui  *cli.MockUi

		username string
		password string
		args     []string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}
		cmd = &LoginCommand{UI: ui}

		username = "iamclovin"
		password = "man u 4evar!!1!"
	})

	It("implements cli.Command", func() {
		var _ cli.Command = cmd
	})

	Context("when run with username and password flags", func() {
		BeforeEach(func() {
			args = []string{
				"-u", username,
				"-p", password,
			}
		})

		It("prints the username and exits successfully", func() {
			exitCode := cmd.Run(args)
			Expect(exitCode).To(Equal(0))
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Logged in as '%s'", username))
		})

		Context("with username flag of the form '-uusername'", func() {
			BeforeEach(func() {
				args = []string{
					fmt.Sprintf("-u%s", username),
					"-p", password,
				}
			})

			It("prints the username and exits successfully", func() {
				exitCode := cmd.Run(args)
				Expect(exitCode).To(Equal(0))
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Logged in as '%s'", username))
			})
		})

		Context("with username flag of the form '--user=username'", func() {
			BeforeEach(func() {
				args = []string{
					fmt.Sprintf("--user=%s", username),
					"-p", password,
				}
			})

			It("prints the username and exits successfully", func() {
				exitCode := cmd.Run(args)
				Expect(exitCode).To(Equal(0))
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Logged in as '%s'", username))
			})
		})
	})
})
