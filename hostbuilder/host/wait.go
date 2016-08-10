package host

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
)

var tr = http.Transport{
	DisableKeepAlives: true,
	TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
}

// WaitForServer takes a url and tries to retrieve a 200 response from it up
// to len(attempts) times, waiting attempts[n-1] duration after attempt n.
// Note that the final element of attempts can be any value, since sleeping
// after is unnecessary.
func WaitForServer(url string, clientTimeout time.Duration, attempts []time.Duration) bool {
	if len(attempts) == 0 {
		return true
	}

	client := &http.Client{
		Transport: &tr,
		Timeout:   clientTimeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}

	for i, duration := range attempts {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}

			logrus.Warnf("WaitForServer: got non-200 response from server at %q, status: %v", url, resp.StatusCode)
		} else {
			logrus.Warnf("WaitForServer: failed to connect to server at %q, err: %v", url, err)
		}
		if i < len(attempts)-1 {
			time.Sleep(duration)
		}
	}

	logrus.Warnf("WaitForServer: could not connect to server at %q after %d attempts", url, len(attempts))

	return false
}
