package docker

import (
	"net/http"
	"strings"
	"testing"

	dockertest "github.com/fsouza/go-dockerclient/testing"
)

func TestCreate(t *testing.T) {
	requests := []*http.Request{}
	server, _ := dockertest.NewServer("127.0.0.1:0", func(r *http.Request) {
		requests = append(requests, r)
	})
	defer server.Stop()

	host, _ := NewHost(
		map[string]string{"endpoint": server.URL()},
		`{"name":"test-123","image":"ubuntu","public_keys":["ssh-rsa 3456"]}`,
	)

	info, err := host.Create()
	if err != nil {
		t.Fatal("Failed to create host", err)
	}
	// assert that an image is created
	{
		r := requests[0]
		if r.URL.Path != "/build" ||
			r.URL.Query().Get("t") != "registry.nitrous.io/test-123" {
			t.Fatal("Incorrect image creation")
		}
	}
	// assert that a container is created
	{
		r := requests[1]
		if r.URL.Path != "/containers/create" ||
			r.URL.Query().Get("name") != "test-123" {
			t.Fatal("Incorrect container creation")
		}
	}
	// assert that container is started
	{
		r := requests[2]
		if r.URL.Path != "/containers/"+info.Id+"/start" {
			t.Fatal("Incorrect container start")
		}
	}
	// assert that container info is called and
	{
		r := requests[3]
		if r.URL.Path != "/containers/"+info.Id+"/json" {
			t.Fatal("Incorrect container info")
		}
	}
	// we return get the ip address of the container
	{
		if info.Provider != "docker" ||
			len(info.Id) == 0 ||
			// hard-coded dockerclient testing ip
			strings.Index(info.IpAddress, "172.16.42") == -1 {
			t.Fatal("Incorrect container info", info)
		}
	}
}
