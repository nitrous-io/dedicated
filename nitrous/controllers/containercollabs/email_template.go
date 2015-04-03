package containercollabs

// {{ .SharerName }} has shared container {{ .ContainerName }} with you!

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
    <p>You will be able to collaborate together through the IDE as well as control the container yourself.</p>
    <p>You can view this container at <a href="{{ .ViewUrl }}" target="_blank">{{ .ViewUrl }}</a>, otherwise look out for it under the Shared area in the sidebar.</p>
    <br />
    <p>Thanks,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const addedTxtTmpl = `
Hello {{ .Name }}!
You have been added as a collaborator on {{ .ContainerSlug }} by {{ .SharerName }}.
You will be able to collaborate together through the IDE as well as control the container yourself.
You can view this container at {{ .ViewUrl }}, otherwise look out for it under the Shared area in the sidebar.

Thanks,
The Nitrous team
`

var (
	addedHtmlTemplate *template.Template
	addedTxtTemplate  *template.Template
)

func init() {
	addedHtmlTemplate =
		template.Must(template.New("addedHtmlTmpl").Parse(addedHtmlTmpl))

	addedTxtTemplate =
		template.Must(template.New("addedTxtTmpl").Parse(addedTxtTmpl))
}
