package mailer

type Mailer interface {
	SendMail(from string, tos []string, ccs []string, bccs []string, replyTo, subject, body, htmltext string) error
}
