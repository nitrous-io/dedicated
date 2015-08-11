package host

import "text/template"

var source = `deb https://{{.AptUsername}}:{{.AptPassword}}@{{.AptLocation}} {{.AptDist}} main`

var SourceTemplate *template.Template

const SourceLocation = "/etc/apt/sources.list.d/nitrousio.list"

func init() {
	SourceTemplate = template.Must(template.New("sourceTemplate").Parse(source))
}
