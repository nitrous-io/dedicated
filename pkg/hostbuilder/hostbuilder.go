package hostbuilder

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type Hostbuilder struct {
	URL            string
	requestHandler func(path string, provider string, config *map[string]sql.NullString) (map[string]string, error)
}

func NewHostbuilder(url string) *Hostbuilder {
	return &Hostbuilder{URL: url}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *map[string]sql.NullString) (map[string]string, error) {
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
		var m map[string]string
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

func (hb *Hostbuilder) OverrideRequestHandler(fn func(path string, provider string, config *map[string]sql.NullString) (map[string]string, error)) {
	hb.requestHandler = fn
}

func (hb *Hostbuilder) getRequestHandler() func(path string, provider string, config *map[string]sql.NullString) (map[string]string, error) {
	rh := hb.requestHandler
	if rh == nil {
		rh = hb.defaultRequestHandler
	}
	return rh
}

func (hb *Hostbuilder) Create(provider string, config map[string]sql.NullString) (map[string]string, error) {
	return hb.getRequestHandler()("", provider, &config)
}

func (hb *Hostbuilder) Delete(provider string, config map[string]sql.NullString) (map[string]string, error) {
	return hb.getRequestHandler()("delete", provider, &config)
}

func (hb *Hostbuilder) Start(provider string, config map[string]sql.NullString) (map[string]string, error) {
	return hb.getRequestHandler()("start", provider, &config)
}

func (hb *Hostbuilder) Stop(provider string, config map[string]sql.NullString) (map[string]string, error) {
	return hb.getRequestHandler()("stop", provider, &config)
}

func (hb *Hostbuilder) Restart(provider string, config map[string]sql.NullString) (map[string]string, error) {
	return hb.getRequestHandler()("restart", provider, &config)
}
