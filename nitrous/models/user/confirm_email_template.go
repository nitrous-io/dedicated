package user

import "text/template"

const confirmEmailNewUserHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Welcome to Nitrous!</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>Thanks for signing up for Nitrous! Before you start developing your first application, please verify your email address by clicking the link below.</p>
    <p><a href="{{ .ConfirmUrl }}" target="_blank">Confirm email</a></p>
    <p>This link will expire in 24 hours if it's not activated.</p>
    <p>Start your engines,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const confirmEmailNewUserTxtTmpl = `
Hello {{ .Name }}!
Thanks for signing up for Nitrous! Before you start developing your first application, please verify your email address by clicking the link below.
{{ .ConfirmUrl }}
This link will expire in 24 hours if it's not activated.

Start your engines,
The Nitrous team
`

const confirmEmailHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - confirm your email</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>To continue using Nitrous we need you to confirm your email address using the link below.</p>
    <p><a href="{{ .ConfirmUrl }}" target="_blank">Confirm email</a></p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const confirmEmailTxtTmpl = `
Hello {{ .Name }}!
To continue using Nitrous we need you to confirm your email address using the link below.
{{ .ConfirmUrl }}

Thanks,
The Nitrous team
`

var (
	confirmEmailNewUserHtmlTemplate *template.Template
	confirmEmailNewUserTxtTemplate  *template.Template

	confirmEmailHtmlTemplate *template.Template
	confirmEmailTxtTemplate  *template.Template
)

func init() {
	confirmEmailNewUserHtmlTemplate =
		template.Must(template.New("confirmEmailNewUserHtml").Parse(confirmEmailNewUserHtmlTmpl))

	confirmEmailNewUserTxtTemplate =
		template.Must(template.New("confirmEmailNewUserTxt").Parse(confirmEmailNewUserTxtTmpl))

	confirmEmailHtmlTemplate =
		template.Must(template.New("confirmEmailHtml").Parse(confirmEmailHtmlTmpl))

	confirmEmailTxtTemplate =
		template.Must(template.New("confirmEmailTxt").Parse(confirmEmailTxtTmpl))
}
