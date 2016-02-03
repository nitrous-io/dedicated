package containercollabs

import "text/template"

const addedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>{{ .Subject }}</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>You have been added as a collaborator on {{ .ContainerSlug }} by {{ .SharerName }}.</p>
    <p>You will be able to collaborate together in the IDE.</p>
    <p>You can view this project at <a href="{{ .ViewUrl }}" target="_blank">{{ .ViewUrl }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const addedTxtTmpl = `
Hello {{ .Name }}!
You have been added as a collaborator on {{ .ContainerSlug }} by {{ .SharerName }}.
You will be able to collaborate together in the IDE.
You can view this project at {{ .ViewUrl }}.

Thanks,
The Nitrous team
`

const invitedHtmlTmpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>{{ .Subject }}</title>
  </head>
  <body>
    <h3>Hello {{ .Email }}!</h3>
    <p>You have been added as a collaborator on {{ .ContainerSlug }} by {{ .SharerName }}.</p>
    <p>You will be able to collaborate in the IDE.</p>
    <p>To begin collaborating on this project, complete your Nitrous signup at <a href="{{ .SignupUrl }}" target="_blank">{{ .SignupUrl }}</a>.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const invitedTxtTmpl = `
Hello {{ .Email }}!
You have been added as a collaborator on {{ .ContainerSlug }} by {{ .SharerName }}.
You will be able to collaborate together through the IDE.
To begin collaborating on this project, complete your Nitrous signup at {{ .SignupUrl }}.

Thanks,
The Nitrous team
`

var (
	addedHtmlTemplate *template.Template
	addedTxtTemplate  *template.Template

	invitedHtmlTemplate *template.Template
	invitedTxtTemplate  *template.Template
)

func init() {
	addedHtmlTemplate =
		template.Must(template.New("addedHtmlTmpl").Parse(addedHtmlTmpl))

	addedTxtTemplate =
		template.Must(template.New("addedTxtTmpl").Parse(addedTxtTmpl))

	invitedHtmlTemplate =
		template.Must(template.New("invitedHtmlTmpl").Parse(invitedHtmlTmpl))

	invitedTxtTemplate =
		template.Must(template.New("invitedTxtTmpl").Parse(invitedTxtTmpl))
}
