package docker

import "text/template"

const dockerfileTmpl = `
FROM nitrousio/nitrous-ubuntu-dind:14.04-1.7.0-nitrous1

ENV HOME /root

RUN mkdir -p /etc/ssh
RUN touch /etc/ssh/univ_public_keys
{{ range .PublicKeys }}
RUN echo -e "{{.}}" >> /etc/ssh/univ_public_keys
{{ end }}

RUN mkdir -p /etc/freighterd

{{ if .TLS }}
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.CA}} > {{.Paths.CA}}'
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.Cert}} > {{.Paths.Cert}}'
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.Key}} > {{.Paths.Key}}'
{{ end }}

{{ if .Secret }}
RUN /bin/bash -c 'echo -e {{printf "%q" .Secret}} > {{.Paths.Secret}}'
{{ end }}

{{ if .Config }}
RUN /bin/bash -c 'echo -e {{printf "%q" .Config}} > {{.Paths.Config}}'
{{ end }}

RUN /bin/bash -c "echo -e '#!/bin/bash\nrm /var/run/docker.pid\ndocker -d >& /var/log/docker.log &\n/freighterd' > /start-freighterd" && chmod +x /start-freighterd

EXPOSE 65333
`

type paths struct {
	CA     string
	Cert   string
	Key    string
	Secret string
	Config string
}

type templateData struct {
	*Host
	Config string
	Paths  paths
}

var dockerfileTemplate *template.Template

func init() {
	dockerfileTemplate =
		template.Must(template.New("dockerfile").Parse(dockerfileTmpl))
}

func newTemplateData(h *Host) *templateData {
	return &templateData{
		Host: h,
		Paths: paths{
			CA:     TLSCALocation,
			Cert:   TLSCertLocation,
			Key:    TLSKeyLocation,
			Secret: SecretLocation,
			Config: ConfigLocation,
		},
	}
}
