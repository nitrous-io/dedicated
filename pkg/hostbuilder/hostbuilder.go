package hostbuilder

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type Config map[string]interface{}
type Output map[string]interface{}

type RequestHandler func(path string, provider string, config *Config) (Output, error)

type Hostbuilder struct {
	URL            string
	requestHandler RequestHandler
}

func NewHostbuilder(url string) *Hostbuilder {
	return &Hostbuilder{URL: url}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *Config) (Output, error) {
	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}
	client := &http.Client{Transport: tr}

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	var url string
	if path == "" {
		url = hb.URL + "/hosts"
	} else {
		url = hb.URL + "/hosts/" + path
	}

	response, err := client.PostForm(url, p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var m Output
		err = json.NewDecoder(response.Body).Decode(&m)
		if err != nil {
			return nil, err
		}
		return m, nil
	default:
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(string(e))
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

type OperationFn func(provider string, config *Config) (Output, error)

func (hb *Hostbuilder) Create(provider string, config *Config) (Output, error) {
	return hb.getRequestHandler()("", provider, config)
}

func (hb *Hostbuilder) Delete(provider string, config *Config) (Output, error) {
	return hb.getRequestHandler()("delete", provider, config)
}

func (hb *Hostbuilder) Start(provider string, config *Config) (Output, error) {
	return hb.getRequestHandler()("start", provider, config)
}

func (hb *Hostbuilder) Stop(provider string, config *Config) (Output, error) {
	return hb.getRequestHandler()("stop", provider, config)
}

func (hb *Hostbuilder) Restart(provider string, config *Config) (Output, error) {
	return hb.getRequestHandler()("restart", provider, config)
}
