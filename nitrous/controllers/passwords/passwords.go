package passwords

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"net/url"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

var Mailer mailer.Mailer = mailer.NewSendGridMailer()
var (
	errInvalidToken = httperror.New(http.StatusNotFound, "invalid_token", "invalid request - make sure the password reset link is correct")
)

func SendResetPasswordToken(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := r.FormValue("login")
	if login == "" {
		// respond with 200 OK and exit
		respond(env)
		return nil
	}

	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		return err
	}
	if u == nil {
		// respond with 200 OK and exit
		respond(env)
		return nil
	}

	err = u.SetResetPasswordToken(db)
	if err != nil {
		return err
	}

	from := (&mail.Address{Name: "Nitrous", Address: "support@nitrous.io"}).String()
	to := (&mail.Address{Name: u.Name, Address: u.Email}).String()
	replyTo := from
	subject := fmt.Sprintf("[nitrous-pro] Reset password instructions")

	var body bytes.Buffer
	host := os.Getenv("HOST_HOSTNAME")
	if host == "" {
		host = "pro.nitrous.io"
	}
	resetURL := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app",
		Fragment: fmt.Sprintf("/reset-password?token=%s", u.ResetPasswordToken.String),
	}

	name := u.Username
	if u.Name != "" {
		name = u.Name
	}

	t := template.Must(template.New("body").Parse(resetPasswordMailBody))
	err = t.Execute(&body, map[string]string{"Name": name, "URL": resetURL.String()})
	if err != nil {
		return err
	}

	err = Mailer.SendMail(from, []string{to}, nil, nil, replyTo, subject, "", body.String())
	if err != nil {
		return err
	}

	respond(env)
	return nil
}

func ResetPassword(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	token := r.FormValue("reset_token")
	if token == "" {
		return errInvalidToken
	}
	u, err := user.FindByResetPasswordToken(db, token)
	if err != nil {
		return err
	}
	if u == nil {
		return errInvalidToken
	}

	if password := r.FormValue("password"); password != "" {
		u.Password = password
	}

	if err := u.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	if err := u.Update(db); err != nil {
		return err
	}

	respond(env)
	return nil
}

func respond(env *middleware.Env) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
}

const resetPasswordMailBody = `
Hello {{.Name}},

<p>Someone has requested a link to change your password. You can reset your password using the following link:</p>

<a href="{{.URL}}">Change my password</a>

<p>If you didn't request to reset your password, please ignore this email.</p>

<p>Your password won't change until you access the link above and create a new one.</p>
`
