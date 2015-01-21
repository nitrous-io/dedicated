package engagement

import "text/template"

const activateEmailHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Activate your Nitrous account</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>Thanks for confirming your email address. To create your first workspace you need to activate your account by adding your billing details.</p>
    <p>You can update your billing information by clicking the following link:</p>
    <p><a href="{{ .ActivateUrl }}" target="_blank">Activate account</a></p>
    <p>Start your engines,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const activateEmailTxtTmpl = `
Hello {{ .Name }}!
Thanks for confirming your email address. To create your first workspace you need to activate your account by adding your billing details.
You can update your billing information by clicking the following link:
{{ .ActivateUrl }}

Start your engines,
The Nitrous team
`

const getStartedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Nitrous: $5 Credit to Get Started</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>To help you get started developing your first application in the cloud, we're giving you a $5 account credit.</p>
    <p>That’s good for a week free to kick the tires on a SSD cloud server with 1GB RAM and 30GB SSD. All you need to do is click the link below to claim your signup credit.</p>
    <p><a href="{{ .ClaimUrl }}" target="_blank">Claim Credit</a></p>
    <p>Note: This code will not work if you have already applied another signup code to your account.</p>
    <p>Start your engines,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const getStartedEmailTxtTmpl = `
Hello {{ .Name }}!
To help you get started developing your first application in the cloud, we're giving you a $5 account credit.
That’s good for a week free to kick the tires on a SSD cloud server with 1GB RAM and 30GB SSD. All you need to do is click the link below to claim your signup credit.
{{ .ClaimUrl }}
Note: This code will not work if you have already applied another signup code to your account.

Start your engines,
The Nitrous team
`

var (
	activateEmailHtmlTemplate *template.Template
	activateEmailTxtTemplate  *template.Template

	getStartedEmailHtmlTemplate *template.Template
	getStartedEmailTxtTemplate  *template.Template
)

func init() {
	getStartedEmailHtmlTemplate =
		template.Must(template.New("getStartedEmailHtml").Parse(getStartedHtmlTmpl))

	getStartedEmailTxtTemplate =
		template.Must(template.New("getStartedEmailTxt").Parse(getStartedEmailTxtTmpl))

	activateEmailHtmlTemplate =
		template.Must(template.New("activateEmailHtml").Parse(activateEmailHtmlTmpl))

	activateEmailTxtTemplate =
		template.Must(template.New("activateEmailTxt").Parse(activateEmailTxtTmpl))
}
