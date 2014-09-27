package mailer

import (
	"html"
	"log"
	"os"
	"strings"

	"github.com/sendgrid/sendgrid-go"
)

var (
	sendgridUsername string
	sendgridPassword string

	disabled = false
)

func init() {
	sendgridUsername = os.Getenv("SENDGRID_USERNAME")
	if sendgridUsername == "" {
		log.Fatalln("SENDGRID_USERNAME not set")
	} else if strings.HasPrefix(sendgridUsername, "disable") {
		log.Println("Warning: Sendgrid mailer disabled.")
		disabled = true
		return
	}
	sendgridPassword = os.Getenv("SENDGRID_PASSWORD")
	if sendgridPassword == "" {
		log.Fatalln("SENDGRID_PASSWORD not set")
	}
}

type SendGridMailer struct {
	client *sendgrid.SGClient
}

func NewSendGridMailer() *SendGridMailer {
	var cl *sendgrid.SGClient = nil
	if !disabled {
		cl = sendgrid.NewSendGridClient(sendgridUsername, sendgridPassword)
	}
	return &SendGridMailer{client: cl}
}

func (s *SendGridMailer) SendMail(from string, tos []string, ccs []string, bccs []string, replyTo, subject, body, htmltext string) error {
	m := sendgrid.NewMail()
	m.SetFrom(from)

	if tos != nil {
		m.AddTos(tos)
	}
	if ccs != nil {
		m.AddCcs(ccs)
	}
	if bccs != nil {
		m.AddBccs(bccs)
	}

	m.SetReplyTo(replyTo)
	m.SetSubject(subject)
	m.SetText(body)
	if htmltext == "" {
		htmltext = html.EscapeString(body)
		htmltext = "<html><body>" + strings.Replace(htmltext, "\n", "<br>", -1) + "</body></html>"
	}
	m.SetHTML(htmltext)

	if s.client != nil {
		if err := s.client.Send(m); err != nil {
			return err
		}
	}
	return nil
}
