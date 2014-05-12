package digitalocean

import (
	"net/http/httptest"
	"testing"

	do "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
)

var (
	defaultTestConfig = map[string]string{
		"zone_id":        "zone_id",
		"base_domain":    "nitrousbox.com",
		"aws_secret_key": "secret_key",
		"aws_access_key": "access_key",
		"client_id":      "client_id",
		"api_key":        "api_key",
	}
)

func TestStop(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/power_off", dotest.OkResponse)

	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := do.NewClient("client_id", "api_key")
	c.BaseURL = ts.URL
	h := Host{Config: &HostConfig{Id: "1"}, Client: c}

	err := h.Stop()
	if err != nil {
		t.Fatal("Failed to stop the host")
	}

	request := s.Requests["droplets/id/power_off"][0]
	if request.URL.Path != "/droplets/1/power_off" {
		t.Fatal("invalid poweroff url", request.URL)
	}
}

func TestStart(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/power_on", dotest.OkResponse)

	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := do.NewClient("client_id", "api_key")
	c.BaseURL = ts.URL
	h := Host{Config: &HostConfig{Id: "1"}, Client: c}

	err := h.Start()
	if err != nil {
		t.Fatal("Failed to start the host")
	}

	request := s.Requests["droplets/id/power_on"][0]
	if request.URL.Path != "/droplets/1/power_on" {
		t.Fatal("invalid power_on url", request.URL)
	}
}
