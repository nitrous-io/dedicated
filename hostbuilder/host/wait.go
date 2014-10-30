package host

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/nitrous-io/nitrous-common/httpclient"
)

// WaitForServer takes a url and tries to retrieve a 200 response from it up
// to len(attempts) times, waiting attempts[n-1] duration after attempt n.
// Note that the final element of attempts can be any value, since sleeping
// after is unnecessary.
func WaitForServer(url string, clientTimeout time.Duration, attempts []time.Duration) bool {
	if len(attempts) == 0 {
		return true
	}

	client := httpclient.New("")
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	client.Client.Transport = tr
	client.Client.Timeout = clientTimeout
	for i, duration := range attempts {
		resp, err := client.Get(url, nil)
		if err == nil && resp.StatusCode == http.StatusOK {
			return true
		}
		if i < len(attempts)-1 {
			time.Sleep(duration)
		}
	}
	return false
}
