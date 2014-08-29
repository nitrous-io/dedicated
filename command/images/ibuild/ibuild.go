package ibuild

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"
	flag "github.com/ogier/pflag"
)

const (
	tagUsage = "give image a name after building (automatically generated if omitted)"
	Usage    = `
Usage: nitro images:build <host> <path to Dockerfile or tarball with Dockerfile>

  build image on host.  If a tarball is provided, it must have a Dockerfile in the archive root

Options:

  -t, --tag=""    %s
`
)

var (
	errInvalidTar = fmt.Errorf("Invalid tar file: must contain a file named 'Dockerfile' in archive root")
)

type Command struct {
	UI     command.UI
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("images:build", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     command.DefaultUI,
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(fmt.Sprintf(Usage, tagUsage))
}

func (c *Command) Run(args []string) int {
	var tag string
	cmdFlags := flag.NewFlagSet("images:build", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&tag, "tag", "t", "", tagUsage)

	err := cmdFlags.Parse(args)
	if err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		c.UI.Output(c.Help())
		return 1
	}

	host, fpath := args[0], args[1]
	if tag == "" {
		tag = randomnamegenerator.Generate("image")
	}

	c.UI.Output(fmt.Sprintf("Building image on '%s' from %s ...", host, fpath))

	if err := c.makeRequest(host, fpath, tag); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Output(fmt.Sprintf("Built image '%s' on '%s'", tag, host))
	return 0
}

func (c *Command) Synopsis() string {
	return "Build image on a host"
}

func (c *Command) makeRequest(host, fpath, tag string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	r, err := loadFile(fpath)
	if err != nil {
		return err
	}

	part, err := writer.CreateFormFile("file", "buildcontext.tar")
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, r); err != nil {
		return err
	}
	if err := writer.WriteField("slug", tag); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	path := fmt.Sprintf("%s/api/v0/hosts/%s/images", c.Server, host)
	req, err := http.NewRequest("POST", path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.Client.DoAuthed(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return httpclient.CheckResponse(resp, http.StatusOK)
}

// loadFile returns a reader to path if path is a tarball, and wraps path in a
// tarball if path is a Dockerfile
func loadFile(fpath string) (io.Reader, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// assumption: file named Dockerfile is not a tar; all other files are tars
	if path.Base(fpath) == "Dockerfile" {
		return dockerfileTar(f)
	} else {
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		if !isValidTar(bytes.NewBuffer(b)) {
			return nil, errInvalidTar
		}
		return bytes.NewBuffer(b), nil
	}
}

// takes a Dockerfile and returns a tar Reader
func dockerfileTar(f *os.File) (io.Reader, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	hdr := &tar.Header{
		Name: "Dockerfile",
		Size: fi.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := io.Copy(tw, f); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

func isValidTar(r io.Reader) bool {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err != nil {
			return false
		}
		if hdr.Name == "Dockerfile" || hdr.Name == "./Dockerfile" {
			return true
		}
	}
}
