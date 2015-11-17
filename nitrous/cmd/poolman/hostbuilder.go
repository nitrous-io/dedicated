package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/nitrous-io/nitrous-common/httpclient"
)

// parses the /pools response from Hostbuilder into a map of region -> (type -> count)
func parseList(body []byte) (map[string]map[string]int, error) {
	var r map[string]map[string]int
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r, nil
}

type HBClient struct {
	URL string
	*httpclient.Client
}

func NewHBClient(url string) *HBClient {
	client := httpclient.New("")
	client.Transport = &http.Transport{
		DisableKeepAlives: true,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
	}
	client.Timeout = 10 * time.Minute
	return &HBClient{
		URL:    url,
		Client: client,
	}
}

func (hb HBClient) List() ([]byte, error) {
	endpoint := fmt.Sprintf("%s/pools", hb.URL)
	p := url.Values{}
	p.Set("provider", "amazon")

	response, err := hb.Get(endpoint, &p)
	if err != nil {
		return nil, fmt.Errorf("hb list error: %v", err)
	}

	defer response.Body.Close()
	return ioutil.ReadAll(response.Body)
}

func (hb HBClient) Grow(params url.Values) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/pools/grow", hb.URL)
	params.Set("provider", "amazon")

	response, err := hb.PostForm(endpoint, &params)
	if err != nil {
		return nil, fmt.Errorf("hb grow error: %v", err)
	}

	defer response.Body.Close()
	return ioutil.ReadAll(response.Body)
}
