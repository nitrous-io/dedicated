package freequeue

// {{ .SharerName }} has shared container {{ .ContainerName }} with you!

import "text/template"

const approvedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>{{ .Subject }}</title>
  </head>
  <body>
    <h3>Great news {{ .Name }}!</h3>
    <p>Your free Nitrous workspace is ready to create at <a href="{{ .ActionUrl }}" target="_blank">{{ .ActionUrl }}</a>.</p>
    <p>Click the link above to activate your workspace and begin developing!</p>
    <p>If you have any questions please email our support team at <a href="mailto:{{ .SupportEmail }}">{{ .SupportEmail }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const approvedTxtTmpl = `
Great news {{ .Name }}!
Your free Nitrous workspace is ready to create at {{ .ActionUrl }}.
Click the link above to activate your workspace and begin developing!
If you have any questions please email our support team at {{ .SupportEmail }}.

Thanks,
The Nitrous team
`

var (
	approvedHtmlTemplate *template.Template
	approvedTxtTemplate  *template.Template
)

func init() {
	approvedHtmlTemplate =
		template.Must(template.New("approvedHtmlTmpl").Parse(approvedHtmlTmpl))

	approvedTxtTemplate =
		template.Must(template.New("approvedTxtTmpl").Parse(approvedTxtTmpl))
}
