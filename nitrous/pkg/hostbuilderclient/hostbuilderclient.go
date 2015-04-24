package hostbuilderclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/nitrous-io/dedicated/hostbuilder"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

type Config map[string]interface{}

type Hostbuilder struct {
	URL    string
	Client *httpclient.Client
}

func NewHostbuilder(url string) *Hostbuilder {
	client := httpclient.New("")
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client.Client.Transport = tr
	return &Hostbuilder{URL: url, Client: client}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config Config, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	var endpoint string
	if path == "" {
		endpoint = hb.URL + "/hosts"
	} else {
		endpoint = hb.URL + "/hosts/" + path
	}
	endpoint += "?" + urlArgs.Encode()

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to encode config: %s", err)
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	response, err := hb.Client.PostFormWithHeaders(endpoint, &p, headers)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to post: %s, err: %s", endpoint, err)
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		m := &hostbuilder.Response{}
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

// OperationFn defines an action that can be invoked on hostbuilder
type OperationFn func(provider string, config Config) (*hostbuilder.Response, error)

// Create sends POST /hosts request to hostbuilder and stream response back to
// the provided writer
func (hb *Hostbuilder) Create(provider string, config Config, out io.Writer, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	endpoint := hb.URL + "/hosts"
	return hb.stream(endpoint, provider, config, out, headers, urlArgs)
}

func (hb *Hostbuilder) Update(provider string, config Config, out io.Writer, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	endpoint := hb.URL + "/hosts/update"
	return hb.stream(endpoint, provider, config, out, headers, urlArgs)
}

func (hb *Hostbuilder) Delete(provider string, config Config, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("delete", provider, config, headers, urlArgs)
}

func (hb *Hostbuilder) Start(provider string, config Config, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("start", provider, config, headers, urlArgs)
}

func (hb *Hostbuilder) Stop(provider string, config Config, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("stop", provider, config, headers, urlArgs)
}

func (hb *Hostbuilder) Restart(provider string, config Config, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("restart", provider, config, headers, urlArgs)
}

func (hb *Hostbuilder) List(provider string) ([]*hostbuilder.Response, error) {
	endpoint := hb.URL + "/hosts"
	p := &url.Values{}
	p.Set("provider", provider)

	response, err := hb.Client.Get(endpoint, p)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to list hosts, err: %s", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, err)
		}
		return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, string(e))
	}

	var j struct {
		Hosts []*hostbuilder.Response `json:"hosts"`
	}
	if err := json.NewDecoder(response.Body).Decode(&j); err != nil {
		return nil, fmt.Errorf("hostbuilder failed to decode: %s response, err: %s", endpoint, err)
	}
	return j.Hosts, nil
}

func (hb *Hostbuilder) stream(endpoint, provider string, config Config, out io.Writer, headers http.Header, urlArgs url.Values) (*hostbuilder.Response, error) {
	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to encode config: %s", err)
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	endpoint += "?" + urlArgs.Encode()
	response, err := hb.Client.PostFormWithHeaders(endpoint, &p, headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	pr, pw := io.Pipe()
	combined := io.MultiWriter(out, pw) // Tee response to output stream and pipe.

	// Stream hostbuilder progress to output streams.
	writeErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(combined, response.Body)
		writeErrCh <- err
		pw.Close()
	}()

	// Check if JSON stream contains an error and return it as an error.
	decoder := json.NewDecoder(pr)
	for {
		msg := &hostbuilder.Response{}
		err := decoder.Decode(msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("hostbuilder: failed to decode stream: %s", err)
		}
		if msg.Error != "" {
			return msg, fmt.Errorf("hostbuilder: failed with error: %s", msg.Error)
		}
		if msg.ID != "" {
			return msg, nil
		}
	}
	return nil, errors.New("hostbuilder: failed to return a proper host")
}
