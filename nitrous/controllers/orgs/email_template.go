package orgs

import "text/template"

const invitedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>{{ .Subject }}</title>
  </head>
  <body>
    <h3>Hello!</h3>
    <p>{{ .OwnerName }} invited you to join the {{ .OrgName }} organization on Nitrous.</p>
    <p>You will be able to create projects under this organization.</p>
    <p>Complete your Nitrous signup at <a href="{{ .SignupUrl }}" target="_blank">{{ .SignupUrl }}</a>.</p>
 
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const invitedTxtTmpl = `
Hello!
{{ .OwnerName }} invited you to join the {{ .OrgName }} organization on Nitrous.

You will be able to create projects under this organization.
Complete your Nitrous signup at {{ .SignupUrl }}.
 
Thanks,
The Nitrous team
`

var (
	invitedHtmlTemplate *template.Template
	invitedTxtTemplate  *template.Template
)

func init() {
	invitedHtmlTemplate =
		template.Must(template.New("invitedHtmlTmpl").Parse(invitedHtmlTmpl))

	invitedTxtTemplate =
		template.Must(template.New("invitedTxtTmpl").Parse(invitedTxtTmpl))
}
