package test

type FakeMailer struct {
	SendMailCalled bool
	From           string
	Tos            []string
	Ccs            []string
	Bccs           []string
	ReplyTo        string
	Subject        string
	Body           string
	HTML           string
	Error          error
}

func (s *FakeMailer) SendMail(from string, tos []string, ccs []string, bccs []string, replyTo, subject, body, htmltext string) error {
	s.SendMailCalled = true
	s.From = from
	s.Tos = tos
	s.Ccs = ccs
	s.Bccs = bccs
	s.ReplyTo = replyTo
	s.Subject = subject
	s.Body = body
	s.HTML = htmltext
	return s.Error
}
