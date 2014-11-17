package user

import "text/template"

const resetPaswordEmailHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - reset password</title>
  </head>
  <body>
    <p>Hello {{.Name}}</p>
		<p>Someone has requested a link to change your password. You can reset your password using the following link:</p>
		<a href="{{.URL}}">Change my password</a>
		<p>If you didn't request to reset your password, please ignore this email.</p>
		<p>Your password won't change until you access the link above and create a new one.</p>
  </body>
</html>
`

const resetPasswordEmailTxtTmpl = `
Hello {{.Name}}
Someone has requested a link to change your password. You can reset your password using the following link:

{{.URL}}

If you didn't request to reset your password, please ignore this email.

Your password won't change until you access the link above and create a new one.
`

var (
	resetPasswordEmailHtmlTemplate *template.Template
	resetPasswordEmailTxtTemplate  *template.Template
)

func init() {
	resetPasswordEmailHtmlTemplate =
		template.Must(template.New("resetPasswordEmailHtml").Parse(resetPaswordEmailHtmlTmpl))

	resetPasswordEmailTxtTemplate =
		template.Must(template.New("resetPasswordEmailTxt").Parse(resetPasswordEmailTxtTmpl))
}
