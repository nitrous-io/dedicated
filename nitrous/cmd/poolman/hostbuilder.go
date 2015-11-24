package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/nitrous-common/httpclient"
)

// number of consecutive fails allowed before growPool shuts down
const maxConsecFails = 5

// growStatus is a simple way of preventing unlimited grows if calls to grow
// endpoint repeatedly fail
type growStatus struct {
	mu    sync.Mutex
	fails int
}

func (g *growStatus) Inc() int {
	g.mu.Lock()
	g.fails++
	fails := g.fails
	g.mu.Unlock()
	return fails
}

func (g *growStatus) Reset() {
	g.mu.Lock()
	g.fails = 0
	g.mu.Unlock()
}

func (g *growStatus) Val() int {
	g.mu.Lock()
	fails := g.fails
	g.mu.Unlock()
	return fails
}

// parses the /pools response from Hostbuilder into a map of region -> (type -> count)
func parseList(body []byte) (map[string]map[string]int, error) {
	var r map[string]map[string]int
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r, nil
}

// meant to be called as goroutine; errors logged instead of returned
func growPool(s *growStatus, region, itype string) {
	fails := s.Val()
	f := logrus.Fields{
		"func":          "growPool",
		"region":        region,
		"instance_type": itype,
		"fails":         fails,
	}
	if fails >= maxConsecFails {
		logrus.WithFields(f).Errorf("consecutive fails reached. Growing halted till restart")
		return
	}
	var rc regionConfig
	for _, reg := range appConfig.Regions {
		if reg.ID == region {
			rc = reg
			break
		}
	}
	if rc.ID == "" {
		logrus.WithFields(f).Errorf("region not found")
		return
	}
	config := map[string]string{
		"region_id":         rc.ID,
		"instance_type":     itype,
		"source_ami":        rc.SourceAMI,
		"subnet_id":         rc.SubnetID,
		"security_group_id": rc.SecurityGroupID,
	}
	cjson, err := json.Marshal(config)
	if err != nil {
		logrus.WithFields(f).Errorf("error marshalling json: %v", err)
		return
	}
	params := url.Values{}
	params.Set("config", string(cjson))
	bs, err := hbClient.Grow(params)
	if err != nil {
		f["fails"] = s.Inc()
		logrus.WithFields(f).Errorf("grow call failed: %v", err)
		return
	} else {
		s.Reset()
	}
	logrus.WithFields(f).Infof("hostbuilder response: %s", string(bs))
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

	logrus.Debugf("calling %s with params %+v", endpoint, params)
	response, err := hb.PostForm(endpoint, &params)
	if err != nil {
		return nil, fmt.Errorf("hb grow error: %v", err)
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		if strings.Contains(string(body), "maximum instances in pool reached") {
			return body, nil
		}
		return body, fmt.Errorf("unsuccessful response")
	}
	return body, nil
}
