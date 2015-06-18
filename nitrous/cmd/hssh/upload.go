package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/cli"
)

type uploadCmd struct {
}

func newUploadCommand() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &uploadCmd{}, nil
	}
}

type uploadArg struct {
	f      io.Reader
	local  string
	remote string
}

func parseUpload(spec string) (*uploadArg, error) {
	arr := strings.Split(spec, ":")
	if len(arr) != 2 {
		return nil, fmt.Errorf("invalid upload %q", spec)
	}

	local, remote := arr[0], arr[1]
	local = filepath.Clean(local)
	f, err := os.Open(local)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q, error: %q", local, err)
	}

	return &uploadArg{
		local:  local,
		f:      f,
		remote: filepath.Clean(remote),
	}, nil
}

func (s *uploadCmd) Run(args []string) int {
	if len(args) < 2 {
		return failf("invalid arguments")
	}

	slug := args[0]
	uploads := []*uploadArg{}
	for _, arg := range args[1:] {
		u, err := parseUpload(arg)
		if err != nil {
			return failf("invalid upload path %q, error: %q", arg, err)
		}
		uploads = append(uploads, u)
	}

	conn, err := newConn(slug)
	if err != nil {
		return failf("upload: %q", err)
	}

	for _, u := range uploads {
		if err := conn.Upload(u.remote, u.f); err != nil {
			return failf("failed to upload %q to %s:%q", u.local, slug, u.remote)
		}
		logrus.Infof("uploaded %q to %s:%q", u.local, slug, u.remote)
	}

	return 0
}

func (s *uploadCmd) Help() string {
	return "hssh upload remote-host-slug file:/remotepath ..."
}

func (s *uploadCmd) Synopsis() string {
	return "Upload files to remote host"
}
