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
