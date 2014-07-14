package hostbuilderclient

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

type Config map[string]interface{}

type RequestHandler func(path string, provider string, config *Config) (hostbuilder.Response, error)

type Hostbuilder struct {
	URL            string
	requestHandler RequestHandler
}

func NewHostbuilder(url string) *Hostbuilder {
	return &Hostbuilder{URL: url}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *Config) (hostbuilder.Response, error) {
	client := httpclient.New("")
	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}
	client.Client.Transport = tr

	output := hostbuilder.Response{}

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return output, err
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

	response, err := client.PostForm(url, &p)
	if err != nil {
		return output, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		err = json.NewDecoder(response.Body).Decode(&output)
		if err != nil {
			return output, err
		}
		return output, nil
	default:
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return output, err
		}
		return output, errors.New(string(e))
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

type OperationFn func(provider string, config *Config) (hostbuilder.Response, error)

func (hb *Hostbuilder) Create(provider string, config *Config) (hostbuilder.Response, error) {
	return hb.getRequestHandler()("", provider, config)
}

func (hb *Hostbuilder) Delete(provider string, config *Config) (hostbuilder.Response, error) {
	return hb.getRequestHandler()("delete", provider, config)
}

func (hb *Hostbuilder) Start(provider string, config *Config) (hostbuilder.Response, error) {
	return hb.getRequestHandler()("start", provider, config)
}

func (hb *Hostbuilder) Stop(provider string, config *Config) (hostbuilder.Response, error) {
	return hb.getRequestHandler()("stop", provider, config)
}

func (hb *Hostbuilder) Restart(provider string, config *Config) (hostbuilder.Response, error) {
	return hb.getRequestHandler()("restart", provider, config)
}
