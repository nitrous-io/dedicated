package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitrous-common/httpclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLogs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "logs")
}

var _ = Describe("logs", func() {
	It("requires container slug", func() {
		cmd := &Command{
			UI: &cli.BasicUi{ErrorWriter: bytes.NewBuffer(nil)},
		}
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1), "Requires container slug")
	})

	It("get logs of the container", func() {
		ns := newNitrousServer("container-slug")
		defer ns.Close()
		stdout := bytes.NewBuffer(nil)
		cmd := &Command{
			UI:     &cli.BasicUi{Writer: stdout},
			server: ns.srv.URL,
			client: httpclient.New("testtoken"),
		}
		cmd.Run([]string{"container-slug"})
		Expect(stdout.String()).To(Equal("hello im a log"))
	})
})

// nitrous test server
type nitrousServer struct {
	srv         *httptest.Server
	ts          *testServer
	containerID string
}

func (s *nitrousServer) getContainer(w http.ResponseWriter, r *http.Request) {
	addr := s.ts.srv.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.ParseInt(port, 10, 64)
	resp := map[string]interface{}{
		"CID": s.containerID,
		"host": map[string]interface{}{
			"ip_address":      host,
			"freighterd_port": portInt,
		},
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"container": resp,
	})
}

func (s *nitrousServer) Close() {
	s.ts.Close()
	s.srv.CloseClientConnections()
	s.srv.Close()
}

func newNitrousServer(containerID string) *nitrousServer {
	ts := newTestServer(containerID)
	s := &nitrousServer{
		ts:          ts,
		containerID: containerID,
	}
	r := mux.NewRouter()
	r.HandleFunc("/api/v0/containers/"+containerID, s.getContainer).Methods("GET")
	srv := httptest.NewServer(r)
	s.srv = srv
	return s
}

// freighter test server
type testServer struct {
	srv         *httptest.Server
	containerID string
	stdout      io.Reader
}

func (s *testServer) getLogs(w http.ResponseWriter, r *http.Request) {
	io.Copy(w, s.stdout)
}

func (s *testServer) Close() {
	s.srv.CloseClientConnections()
	s.srv.Close()
}

func newTestServer(containerID string) *testServer {
	s := &testServer{
		containerID: containerID,
		stdout:      bytes.NewBufferString("hello im a log"),
	}
	r := mux.NewRouter()
	r.HandleFunc(fmt.Sprintf("/containers/%s/logs", containerID), s.getLogs).Methods("GET")
	srv := httptest.NewServer(r)
	s.srv = srv
	return s
}
