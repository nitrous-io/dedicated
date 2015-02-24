package server

import "os/exec"

type Runner interface {
	Run(cmd *exec.Cmd) ([]byte, error)
}

type realRunner struct{}

func (r *realRunner) Run(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}
