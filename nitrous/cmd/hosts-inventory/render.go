package main

import (
	"os"
	"text/template"

	"github.com/Sirupsen/logrus"
)

var (
	inventoryTmpl *template.Template
)

func init() {
	var err error
	inventoryTmpl, err = template.New("invfile").Parse(inventoryStr)
	if err != nil {
		logrus.Fatalf("Error parsing template: %v", err)
	}
}

func render(f *os.File, hosts []*AnsibleHost) error {
	return inventoryTmpl.Execute(f, map[string]interface{}{"Hosts": hosts})
}

const inventoryStr = `
[userhosts:vars]
ansible_ssh_port=65100
ansible_ssh_user=ubuntu

[userhosts]
{{ range .Hosts }}
{{ .Slug }} ansible_ssh_host={{ .IPAddress.String }} ansible_ssh_private_key_file={{ .FullKeyPath }}
{{ end }}
`
