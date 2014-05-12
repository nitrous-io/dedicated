package digitalocean

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nitrous-io/hostbuilder/pkg/digitalocean/dotest"
)

func TestCreateDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets", dotest.NewResponse(200, allDroplets))
	s.AddResponse("sizes", dotest.NewResponse(200, allSizes))
	s.AddResponse("images/id", dotest.NewResponse(200, oneImage))
	s.AddResponse("regions", dotest.NewResponse(200, allRegions))
	s.AddResponse("droplets/new", dotest.NewResponse(200, newDroplet))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	droplet, err := c.CreateDroplet("test222", "33", "2", "3", 1, false)
	if err != nil || droplet == nil {
		t.Fatal("failed to create new droplet", droplet, err)
	}
	expectedDroplet := &DropletResp{}
	json.Unmarshal([]byte(newDroplet), expectedDroplet)
	if !reflect.DeepEqual(droplet, expectedDroplet.Droplet) {
		t.Fatal("Failed to return correct droplet, expected %v, got %v",
			expectedDroplet.Droplet, newDroplet)
	}
}

func TestDestroyDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/destroy", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.DestroyDroplet(123)
	if err != nil {
		t.Fatal("failed to destroy droplet", err)
	}
	request := s.Requests["droplets/id/destroy"][0]
	if request.URL.Path != "/droplets/123/destroy" {
		t.Fatal("invalid destroy url", request.URL)
	}
}

func TestPoweroffDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/power_off", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.PowerOffDroplet(123)
	if err != nil {
		t.Fatal("failed to power off droplet", err)
	}
	request := s.Requests["droplets/id/power_off"][0]
	if request.URL.Path != "/droplets/123/power_off" {
		t.Fatal("invalid power off url", request.URL)
	}
}

func TestPowerOnDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/power_on", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.PowerOnDroplet(123)
	if err != nil {
		t.Fatal("failed to power on droplet", err)
	}
	request := s.Requests["droplets/id/power_on"][0]
	if request.URL.Path != "/droplets/123/power_on" {
		t.Fatal("invalid power on url", request.URL)
	}
}

func TestPowerCycleDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/power_cycle", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.PowerCycleDroplet(123)
	if err != nil {
		t.Fatal("failed to power cycle droplet", err)
	}
	request := s.Requests["droplets/id/power_cycle"][0]
	if request.URL.Path != "/droplets/123/power_cycle" {
		t.Fatal("invalid url", request.URL)
	}
}

func TestRebootDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/reboot", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.RebootDroplet(123)
	if err != nil {
		t.Fatal("failed to reboot droplet", err)
	}
	request := s.Requests["droplets/id/reboot"][0]
	if request.URL.Path != "/droplets/123/reboot" {
		t.Fatal("invalid url", request.URL)
	}
}

func TestSnapshotDroplet(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/snapshot", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.CreateSnapshot(123, "winterfell-rails-123")
	if err != nil {
		t.Fatal("failed to reboot droplet", err)
	}
	request := s.Requests["droplets/id/snapshot"][0]
	if request.URL.Path != "/droplets/123/snapshot" ||
		request.URL.Query().Get("name") != "winterfell-rails-123" {
		t.Fatal("invalid url", request.URL)
	}
}

func TestRestoreSnapshot(t *testing.T) {
	s := dotest.NewServer()
	s.AddResponse("droplets/id/restore", dotest.OkResponse)
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	c := testClient(ts.URL)
	err := c.RestoreSnapshot(123, 234)
	if err != nil {
		t.Fatal("failed to restore droplet", err)
	}
	request := s.Requests["droplets/id/restore"][0]
	if request.URL.Path != "/droplets/123/restore" ||
		request.URL.Query().Get("image_id") != "234" {
		t.Fatal("invalid url", request.URL)
	}
}
