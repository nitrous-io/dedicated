package docker

import (
	"bytes"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
)

type hook string

const (
	hookPostStart hook = "start"

	hookTimeout = 30 * time.Second
)

func (p *DockerProvider) runLifecycleHook(containerID string, hk hook) error {
	container, err := p.inspectContainer(containerID)
	if err != nil {
		return err
	}

	scripts := p.parseScripts(container)
	script, ok := scripts[string(hk)]
	if !ok {
		// No script defined for given lifecycle stage - return without error.
		return nil
	}

	logrus.Infof("running %q lifecycle hook script: %q", hk, script)

	// TODO Write script's stdout, stderr and exit code to a file in the
	// container (so that user has feedback).
	buf := new(bytes.Buffer)
	exitCode, err := p.RunCommand(containerID, script, hookTimeout, buf)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("%q lifecycle hook script exited with non-zero exit code %d, output: %s", hk, exitCode, buf)
	}

	return nil
}
