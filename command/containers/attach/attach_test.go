package attach_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/containers/attach"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestAttach(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containers/attach")
}

var _ = Describe("containers/attach", func() {
	var (
		// Nitrous Server
		nsrv *ghttp.Server
		// Freighter Server
		fsrv *ghttp.Server
	)

	BeforeEach(func() {
		// Setup Freighter server.
		fsrv = ghttp.NewServer()
		// Setup handlers: attach to container.
		fsrv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Token container-token"),
			ghttp.VerifyRequest("POST", "/containers/deadbeef/attach"),
			func(w http.ResponseWriter, r *http.Request) {
				_, out, _ := hijackServer(w)
				out.Write([]byte("attach output"))
				// Write more bytes than we will read so that io.Copy will fail.
				out.Write([]byte("\n"))
			},
		))

		// Setup nitrous server.
		nsrv = ghttp.NewServer()
		// Setup handlers: request container token.
		nsrv.AppendHandlers(ghttp.CombineHandlers(
			func(w http.ResponseWriter, r *http.Request) {
				r.ParseForm()
				Expect(r.PostForm["permission[]"]).To(Equal([]string{"attach"}))
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/containers/foo-container-42/token"),
			ghttp.RespondWithJSONEncoded(http.StatusOK, map[string]string{"token": "container-token"}),
		))
		// Setup handlers: get container info.
		nsrv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/containers/foo-container-42"),
			ghttp.RespondWithJSONEncoded(
				http.StatusOK,
				map[string]interface{}{
					"container": map[string]interface{}{
						"CID":     "deadbeef",
						"host_id": 1,
						"state":   "running",
					},
				}),
		))
		// Setup handlers: get host info.
		addr := fsrv.HTTPTestServer.Listener.Addr().String()
		fhost, port, _ := net.SplitHostPort(addr)
		fport, err := strconv.ParseInt(port, 10, 64)
		Expect(err).To(BeNil())
		nsrv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/1"),
			ghttp.RespondWithJSONEncoded(http.StatusOK, map[string]interface{}{
				"host": map[string]interface{}{
					"ip_address":      fhost,
					"freighterd_port": fport,
				},
			}),
		))
	})

	AfterEach(func() {
		nsrv.Close()
		fsrv.Close()
	})

	It("requires container slug", func() {
		cmd := &attach.Command{
			UI: &cli.BasicUi{ErrorWriter: bytes.NewBuffer(nil)},
		}
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
	})

	It("attaches to the container", func() {
		stdoutReader, stdout := io.Pipe()
		cmd := &attach.Command{
			UI: &cli.BasicUi{
				Reader:      bytes.NewBuffer(nil),
				Writer:      stdout,
				ErrorWriter: bytes.NewBuffer(nil),
			},
			Server: nsrv.URL(),
			Client: httpclient.New("correct_token"),
		}

		exited := make(chan struct{})
		go func() {
			cmd.Run([]string{"foo-container-42"})
			close(exited)
		}()

		b := make([]byte, len("attach output"))
		_, err := stdoutReader.Read(b)
		Expect(err).To(BeNil())
		Expect(string(b)).To(Equal("attach output"))

		// Close pipe so that the command will exit.
		stdout.Close()
		stdoutReader.Close()

		<-exited

		Expect(nsrv.ReceivedRequests()).To(HaveLen(3))
		Expect(fsrv.ReceivedRequests()).To(HaveLen(1))
	})
})

func hijackServer(w http.ResponseWriter) (io.ReadCloser, io.Writer, error) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: application/vnd.containerd.raw-stream\r\n\r\n")
	return conn, conn, nil
}
