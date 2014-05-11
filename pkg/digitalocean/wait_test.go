package digitalocean

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
)

func TestWaitForStateSuccess(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id", dotest.NewResponse(200, newDropletState))
	s.AddResponse("droplets/id", dotest.NewResponse(200, activeDropletState))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	client := testClient(ts.URL)
	droplet, err := WaitForState("active", 1, client, 1*time.Millisecond, 1*time.Second)
	if err != nil {
		t.Fatalf("failed to wait for state %v", "active")
	}
	if droplet.Status != "active" {
		t.Fatalf("failed to return droplet %v", droplet)
	}
}

func TestWaitForStateTimedout(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id", dotest.NewResponse(200, newDropletState))
	s.AddResponse("droplets/id", dotest.NewResponse(200, activeDropletState))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	client := testClient(ts.URL)
	droplet, err := WaitForState("active", 1, client, 1*time.Millisecond,
		1*time.Millisecond)
	if err == nil || droplet != nil {
		t.Fatal("failed to timeout")
	}
}

func TestWaitForStateFailed(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id", dotest.NewResponse(200, newDropletState))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	client := testClient(ts.URL)
	droplet, err := WaitForState("active", 1, client, 1*time.Millisecond,
		5*time.Millisecond)
	if err == nil || droplet != nil {
		t.Fatal("failed to receive error")
	}
}
