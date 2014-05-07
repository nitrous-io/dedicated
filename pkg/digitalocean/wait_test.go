package digitalocean

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(url string) *Client {
	client := NewClient("client_id", "api_key")
	client.BaseURL = url
	return client
}

func TestWaitForStateSuccess(t *testing.T) {
	respList := []*DropletResp{
		&DropletResp{Status: "OK", Droplet: &Droplet{Status: "new"}},
		&DropletResp{Status: "OK", Droplet: &Droplet{Status: "active"}},
	}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if counter > len(respList)-1 {
			w.WriteHeader(404)
			w.Write([]byte("unexpected request"))
			return
		}
		resp := respList[counter]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(resp)
		counter += 1
	}))
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
	respList := []*DropletResp{
		&DropletResp{Status: "OK", Droplet: &Droplet{Status: "new"}},
		&DropletResp{Status: "OK", Droplet: &Droplet{Status: "active"}},
	}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if counter > len(respList)-1 {
			w.WriteHeader(404)
			w.Write([]byte("unexpected request"))
			return
		}
		resp := respList[counter]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(resp)
		counter += 1
	}))
	defer ts.Close()
	client := testClient(ts.URL)
	droplet, err := WaitForState("active", 1, client, 1*time.Millisecond,
		1*time.Millisecond)
	if err == nil || droplet != nil {
		t.Fatal("failed to timeout")
	}
}

func TestWaitForStateFailed(t *testing.T) {
	respList := []*DropletResp{
		&DropletResp{Status: "OK", Droplet: &Droplet{Status: "new"}},
	}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if counter > len(respList)-1 {
			w.WriteHeader(404)
			w.Write([]byte(`{"status":"ERROR","message":"Not Found"}`))
			return
		}
		resp := respList[counter]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(resp)
		counter += 1
	}))
	defer ts.Close()
	client := testClient(ts.URL)
	droplet, err := WaitForState("active", 1, client, 1*time.Millisecond,
		5*time.Millisecond)
	if err == nil || droplet != nil {
		t.Fatal("failed to receive error")
	}
}
