package user

import "text/template"

const confirmEmailHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous - confirm your email</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>Welcome to Nitrous, we're excited to have you on board.</p>
    <p>To complete the registration process we will need to you to confirm your email, all you have to do is click the link below.</p>
    <p><a href="{{ .ConfirmUrl }}" target="_blank">Confirm email</a></p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const confirmEmailTxtTmpl = `
Hello {{ .Name }}!
Welcome to Nitrous, we're excited to have you on board.
To complete the registration process we will need to you to confirm your email, all you have to do is click the link below.
{{ .ConfirmUrl }}

Thanks,
The Nitrous team
`

var (
	confirmEmailHtmlTemplate *template.Template
	confirmEmailTxtTemplate  *template.Template
)

func init() {
	confirmEmailHtmlTemplate =
		template.Must(template.New("confirmEmailHtml").Parse(confirmEmailHtmlTmpl))

	confirmEmailTxtTemplate =
		template.Must(template.New("confirmEmailTxt").Parse(confirmEmailTxtTmpl))
}
