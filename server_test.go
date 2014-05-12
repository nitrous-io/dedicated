package hostbuilder

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/host/dummy"
)

type webhookTest func(t *testing.T, data WebhookData)

func webhookTestServer(wg sync.WaitGroup, t *testing.T, f webhookTest) *httptest.Server {
	wg.Add(1)
	wh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		payload := r.Form.Get("payload")
		var data WebhookData
		json.Unmarshal([]byte(payload), &data)
		f(t, data)

		fmt.Println(wg)
		wg.Done()
		fmt.Println(wg)
	}))
	return wh
}

func TestCreateHost(t *testing.T) {
	var wg sync.WaitGroup
	wh := webhookTestServer(wg, t, func(t *testing.T, data WebhookData) {
		if data.Success != true {
			t.Fatal("Failed to create a dummy host")
		}
		var info host.Info
		json.Unmarshal([]byte(data.Message), &info)
		if info.Id != "123" || info.IpAddress != "10.0.0.1" {
			t.Fatal("Failed to return correct host info", info, data)
		}
	})
	defer wh.Close()

	s, _ := New([]byte(`{"dummy":{}}`))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	config, _ := json.Marshal(dummy.Host{Id: "123", IpAddress: "10.0.0.1", CreateSuccess: true})
	form := url.Values{}
	form.Set("provider", "dummy")
	form.Set("config", string(config))
	form.Set("webhook", string(wh.URL))
	http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host"), form)

	wg.Wait()
}

func TestDestroyHost(t *testing.T) {
	var wg sync.WaitGroup
	wh := webhookTestServer(wg, t, func(t *testing.T, data WebhookData) {
		if data.Success != true {
			t.Fatal("Failed to destroy a dummy host", data)
		}
	})
	defer wh.Close()

	s, _ := New([]byte(`{"dummy":{}}`))
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	config, _ := json.Marshal(dummy.Host{DestroySuccess: true})
	form := url.Values{}
	form.Set("provider", "dummy")
	form.Set("webhook", string(wh.URL))
	form.Set("config", string(config))
	http.PostForm(fmt.Sprintf("%s/%s", ts.URL, "host/delete"), form)

	wg.Wait()
}
