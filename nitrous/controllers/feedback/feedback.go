package feedback

import (
	"fmt"
	"net/http"
	"net/mail"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

var Mailer mailer.Mailer

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

func Send(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	subject := r.FormValue("subject")
	body := r.FormValue("body")

	if subject == "" && body == "" {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{"error": "invalid_request"}
		return nil
	}

	from := (&mail.Address{Name: "Nitrous Pro", Address: "support@nitrous.io"}).String()
	to := (&mail.Address{Name: "Nitrous Feedback", Address: "support@nitrous.io"}).String()
	replyTo := (&mail.Address{Name: env.User.Name, Address: env.User.Email}).String()

	subject = fmt.Sprintf("[nitrous-pro] %s", subject)
	body = fmt.Sprintf("Username: %s\nName: %s\nEmail: %s\n%s", env.User.Username, env.User.Name, env.User.Email, body)

	err := Mailer.SendMail(from, []string{to}, []string{replyTo}, nil, replyTo, subject, body, "")
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
	return nil
}
