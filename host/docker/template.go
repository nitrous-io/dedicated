package docker

import "text/template"

const dockerfileTmpl = `
FROM nitrousio/ubuntu-dind:14.04

RUN apt-get update && apt-get install -y --no-install-recommends lxc=1.0.* cgmanager libcgmanager0
RUN /bin/bash -c "echo -e '#!/bin/bash\ndocker -d -e lxc >& /var/log/docker.log &\n/freighterd' > /start-freighterd" && chmod +x /start-freighterd
EXPOSE 65333
`

var dockerfileTemplate *template.Template

func init() {
	dockerfileTemplate =
		template.Must(template.New("dockerfile").Parse(dockerfileTmpl))
}
