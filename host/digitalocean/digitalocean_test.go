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

	// /droplets/new
	newDroplet = `
	{
		"status": "OK",
		"droplet": {
			"id": 100823,
			"name": "test222",
			"image_id": 420,
			"size_id":33,
			"region_id": 1
		}
	}
	`

	// /ssh_keys/new
	newSSH = `
	{
		"status": "OK",
		"ssh_key": {
			"id": 47,
			"name": "my_key",
			"ssh_pub_key": "ssh-rsa key user@host"
		}
	}
	`

	// /regions
	allRegions = `
	{
		"status": "OK",
		"regions": [
			{ "id": 1, "name": "New York 1", "slug": "nyc1" },
			{ "id": 2, "name": "Amsterdam 1", "slug": "ams1" },
			{ "id": 3, "name": "San Francisco 1", "slug": "sfo1" }
		]
	}
	`

	// /sizes
	allSizes = `
	{
		"status": "OK",
		"sizes": [
			{ "id": 33, "name": "512MB", "slug": "512mb" },
			{ "id": 34, "name": "1GB", "slug": "1gb" },
			{ "id": 35, "name": "2GB", "slug": "2gb" },
			{ "id": 36, "name": "4GB", "slug": "4gb" },
			{ "id": 37, "name": "8GB", "slug": "8gb" },
			{ "id": 38, "name": "16GB", "slug": "16gb" }
		]
	}
	`

	// /image/id
	oneImage = `
	{
		"status": "OK",
		"image": {
			"id": 1,
			"name": "Automated Backup",
			"distribution":"Ubuntu"
		}
	}
	`
)

func TestCleanFailDropletsAfterCreate(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("ssh_keys/new", dotest.NewResponse(200, newSSH))
	s.AddResponse("sizes", dotest.NewResponse(200, allSizes))
	s.AddResponse("images/id", dotest.NewResponse(200, oneImage))
	s.AddResponse("regions", dotest.NewResponse(200, allRegions))
	s.AddResponse("droplets/new", dotest.NewResponse(200, newDroplet))
	s.AddResponse("droplets/id", dotest.NotfoundResponse)
	s.AddResponse("droplets/id/destroy", dotest.OkResponse)
	s.AddResponse("ssh_keys/id/destroy", dotest.OkResponse)

	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := do.NewClient("client_id", "api_key")
	c.BaseURL = ts.URL
	h := Host{Config: &HostConfig{Id: "1", ImageId: "1", RegionId: "1", SizeId: "33"}, Client: c}

	_, err := h.Create()
	if err == nil {
		t.Fatal("Create should fail")
	}

	// check that the instance will be destroyed
	{
		request := s.Requests["droplets/id/destroy"][0]
		if request.URL.Path != "/droplets/100823/destroy" {
			t.Fatal("invalid droplets destroy url", request.URL)
		}
	}
	// check that ssh key will be destroyed
	{
		request := s.Requests["ssh_keys/id/destroy"][0]
		if request.URL.Path != "/ssh_keys/47/destroy" {
			t.Fatal("invalid ssh destroy url", request.URL)
		}
	}
}

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

func TestRestart(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/reboot", dotest.OkResponse)

	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := do.NewClient("client_id", "api_key")
	c.BaseURL = ts.URL
	h := Host{Config: &HostConfig{Id: "1"}, Client: c}

	err := h.Restart()
	if err != nil {
		t.Fatal("Failed to restart the host")
	}

	request := s.Requests["droplets/id/reboot"][0]
	if request.URL.Path != "/droplets/1/reboot" {
		t.Fatal("invalid reboot url", request.URL)
	}
}
