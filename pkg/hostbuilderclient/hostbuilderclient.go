package hostbuilderclient

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

type Config map[string]interface{}

type RequestHandler func(path string, provider string, config *Config) (*hostbuilder.Response, error)

type Hostbuilder struct {
	URL            string
	requestHandler RequestHandler
}

func NewHostbuilder(url string) *Hostbuilder {
	return &Hostbuilder{URL: url}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *Config) (*hostbuilder.Response, error) {
	client := httpclient.New("")
	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}
	client.Client.Transport = tr

	var endpoint string
	if path == "" {
		endpoint = hb.URL + "/hosts"
	} else {
		endpoint = hb.URL + "/hosts/" + path
	}

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to encode config: %s", err)
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	response, err := client.PostForm(endpoint, &p)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to post: %s, err: %s", endpoint, err)
	}
	defer response.Body.Close()

	m := &hostbuilder.Response{}
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		err = json.NewDecoder(response.Body).Decode(m)
		if err != nil {
			return nil, fmt.Errorf("hostbuilder failed to decode: %s response, err: %s", endpoint, err)
		}
		return m, nil
	default:
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, err)
		}
		return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, string(e))
	}
}

func (hb *Hostbuilder) OverrideRequestHandler(fn RequestHandler) {
	hb.requestHandler = fn
}

func (hb *Hostbuilder) getRequestHandler() RequestHandler {
	if hb.requestHandler != nil {
		return hb.requestHandler
	}
	return hb.defaultRequestHandler
}

type OperationFn func(provider string, config *Config) (*hostbuilder.Response, error)

func (hb *Hostbuilder) Create(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("", provider, config)
}

func (hb *Hostbuilder) Delete(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("delete", provider, config)
}

func (hb *Hostbuilder) Start(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("start", provider, config)
}

func (hb *Hostbuilder) Stop(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("stop", provider, config)
}

func (hb *Hostbuilder) Restart(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("restart", provider, config)
}
