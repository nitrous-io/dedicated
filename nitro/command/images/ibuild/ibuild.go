package ibuild

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/images"
	"github.com/nitrous-io/dedicated/nitro/pkg/randomnamegenerator"
	"github.com/nitrous-io/nitrous-common/httpclient"
	"github.com/nitrous-io/nitrous-common/isarchive"
	flag "github.com/ogier/pflag"
)

const slugUsage = "A slug for the image (automatically generated if omitted)"

var usage = fmt.Sprintf(`
Usage: nitro images:build <host> <path to Dockerfile or tarball with Dockerfile>

  Build image on host. If a tarball is provided, it must have a Dockerfile in the archive root.

Options:

  -s, --slug=""   %s
`, slugUsage)

var (
	errInvalidTar = fmt.Errorf("Invalid tar file: must contain a file named 'Dockerfile' in archive root")
)

type Command struct {
	UI     *cli.BasicUi
	Server string
	Client *httpclient.Client
}

func init() {
	command.RegisterAuth("images:build", NewCommand)
}

func NewCommand(token string) (cli.Command, error) {
	return &Command{
		UI:     &cli.BasicUi{Reader: os.Stdin, Writer: os.Stdout},
		Server: command.Env.APIURL,
		Client: httpclient.New(token),
	}, nil
}

func (c *Command) Help() string {
	return strings.TrimSpace(usage)
}

func (c *Command) Run(args []string) int {
	var slug string
	cmdFlags := flag.NewFlagSet("images:build", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.UI.Output(c.Help()) }
	cmdFlags.StringVarP(&slug, "slug", "s", "", slugUsage)

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	args = cmdFlags.Args()
	if len(args) != 2 {
		c.UI.Output(c.Help())
		return 1
	}

	stdout, ok := c.UI.Writer.(*os.File)
	if !ok {
		c.UI.Error("Requires a working terminal")
		return 1
	}

	host, fpath := args[0], args[1]
	if slug == "" {
		slug = randomnamegenerator.Generate("image")
	}
	c.UI.Output(fmt.Sprintf("Building image on '%s' from %s ...", host, fpath))

	if err := c.buildImage(host, fpath, slug, stdout, stdout.Fd()); err != nil {
		c.UI.Error(fmt.Sprintf("images:build %s %s failed. Error: %s", host, fpath, err))
		return 1
	}

	c.UI.Output(fmt.Sprintf("Built image '%s' on '%s'", slug, host))
	return 0
}

func (c *Command) Synopsis() string {
	return "Build image on a host"
}

func (c *Command) buildImage(host, fpath, slug string, out io.Writer, termFd uintptr) error {
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
	if err := writer.WriteField("slug", slug); err != nil {
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
	if err := httpclient.CheckResponse(resp, http.StatusOK); err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := images.DisplayJSONMessagesStream(resp.Body, out, termFd, true); err != nil {
		return fmt.Errorf("failed to display build progress: %s", err)
	}
	return nil
}

// loadFile returns a reader to path if path is a tarball,
// or wraps path in a tarball if path is a Dockerfile
func loadFile(fpath string) (io.Reader, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bufRdr := bufio.NewReader(f)
	header, err := bufRdr.Peek(isarchive.MaxHeaderLength)
	// Peek can return err even if header is read and read bytes is shorter than requested
	if err != nil && header == nil {
		return nil, err
	}
	isArchive := isarchive.IsArchive(header)

	b := &bytes.Buffer{}
	_, err = b.ReadFrom(bufRdr)
	if err != nil {
		return nil, err
	}

	// if not archive, make tarball
	if !isArchive {
		return makeArchive(b)
	}

	return b, nil
}

// takes a buffered Dockerfile and returns a tar Reader
func makeArchive(dockerfileBuf *bytes.Buffer) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	now := time.Now()
	hdr := &tar.Header{
		Name:       "Dockerfile",
		Size:       int64(dockerfileBuf.Len()),
		ModTime:    now,
		AccessTime: now,
		ChangeTime: now,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(dockerfileBuf.Bytes()); err != nil {
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
