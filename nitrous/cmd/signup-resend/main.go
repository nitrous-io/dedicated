package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"text/template"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/nitrous-common/logger"
)

const usage = `signup-resend <file with usernames separated by newline>`

var (
	logr    *logger.Logger
	liveRun bool

	resendEmailHtmlTemplate *template.Template
	resendEmailTxtTemplate  *template.Template
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	resendEmailHtmlTemplate =
		template.Must(template.New("resendEmailHtml").Parse(resendEmailHtmlDef))

	resendEmailTxtTemplate =
		template.Must(template.New("resendEmailTxt").Parse(resendEmailTxtDef))
}

func loadUsernames(path string) ([]string, error) {
	usernames := []string{}
	uf, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer uf.Close()

	scanner := bufio.NewScanner(uf)
	for scanner.Scan() {
		usernames = append(usernames, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return usernames, nil
}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	envvars.CheckSendgridVariables()

	if len(os.Args) < 2 {
		logr.FatalExit(usage)
	}

	if len(os.Args) == 3 && os.Args[2] == "--live-run" {
		liveRun = true
	}

	usernames, err := loadUsernames(os.Args[1])
	if err != nil {
		logr.FatalfExit("Error scanning file: %s: %s", os.Args[1], err)
	}

	logr.Debugf("Usernames is %+v", usernames)

	m := mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	for _, name := range usernames {
		logr.Debugf("Handling user: %s", name)
		u, err := user.FindByUsername(dbConn, name)
		if err != nil {
			logr.FatalfExit("Error on name '%s': %s", name, err)
		}
		resetUrl := "" // hereafter resetUrl emptiness used to determine confirmed_at
		if !u.ConfirmedAt.Valid {
			ru := &url.URL{
				Scheme:   "https",
				Host:     "pro.nitrous.io",
				Path:     "/app/",
				Fragment: fmt.Sprintf("/confirm-email?token=%s", u.ConfirmationToken.String),
			}
			resetUrl = ru.String()
			if liveRun {
				_, err = dbConn.Exec(`UPDATE users SET confirmation_sent_at = now() WHERE id = $1;`, u.ID)
			}
		}
		subject := "Confirm your Nitrous email"
		if resetUrl == "" {
			subject = "Welcome to Nitrous Pro"
		}

		tmplData := struct {
			Name       string
			ConfirmUrl string
		}{
			u.DisplayName(),
			resetUrl,
		}
		htmlBuf := bytes.NewBuffer(nil)
		if err := resendEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
			logr.FatalfExit("Error compiling: %s", err)
		}
		txtBuf := bytes.NewBuffer(nil)
		if err := resendEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
			logr.FatalfExit("Error compiling: %s", err)
		}
		from := (&mail.Address{Name: "Nitrous Pro", Address: "mailer@nitrous.io"}).String()
		to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()

		if liveRun {
			err = m.SendMail(from, []string{to}, []string{from}, nil, from, subject, txtBuf.String(), htmlBuf.String())
			if err != nil {
				logr.FatalfExit("Error sending: %s", err)
			}
			time.Sleep(500 * time.Millisecond)
		} else {
			logr.Debugf("Sending from %s to %s, subject %s, txtBuf: %s", from, to, subject, txtBuf.String())
		}
		logr.Debugf("Sent to user: %s", name)
	}
}

const resendEmailHtmlDef = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Problems signing up?</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>Thanks for signing up for Nitrous!</p>
    <p>We had an issue with our e-mail provider yesterday and were unable to contact you earlier.  We apologize for any inconvenience caused.</p>
    <p>The good news is that your account creation did go through successfully.  You will be able to sign in with the same username/password you signed up with yesterday.</p>

    {{ if .ConfirmUrl }}
    <p>Before you start developing your first application, please verify your email address by clicking the link below.</p>
    <p><a href="{{ .ConfirmUrl }}" target="_blank">Confirm email</a></p>
    <p>This link will expire in 24 hours if it's not activated.</p>
    {{ end }}

    <p>Start your engines,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const resendEmailTxtDef = `
Hello {{ .Name }}!

Thanks for signing up for Nitrous!

We had an issue with our e-mail provider yesterday and were unable to contact you earlier.  We apologize for any inconvenience caused.

The good news is that your account creation did go through successfully.  You will be able to sign in with the same username/password you signed up with yesterday.

{{ if .ConfirmUrl }}Before you start developing your first application, please verify your email address by clicking the link below.

{{ .ConfirmUrl }}

This link will expire in 24 hours if it's not activated.{{ end }}

Start your engines,
The Nitrous team
`
