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
	confirmEmailHtmlTemplate *template.Template
	confirmEmailTxtTemplate  *template.Template
)

func init() {
	confirmEmailHtmlTemplate =
		template.Must(template.New("confirmEmailHtml").Parse(confirmEmailHtmlTmpl))

	confirmEmailTxtTemplate =
		template.Must(template.New("confirmEmailTxt").Parse(confirmEmailTxtTmpl))
}
