package provisioner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

// Config is the configuration of a host provisioner.
// It is used for unmarshalling JSON of the form:
// {
//   "username": "host-provisioner",
//   "poll_interval_ms": "5000",
//   "preprovision": [
//     {
//       "driver": "digitalocean",
//       "region": "sgp",
//       "plan": "1gb",
//       "count": 2
//     },
//     {
//       "driver": "docker",
//       "region": "sgp",
//       "plan": "512mb",
//       "count": 1
//     },
//   ]
// }
type Config struct {
	Username          string             `json:"username"`
	PollIntervalMS    int64              `json:"poll_interval_ms"` // In milliseconds.
	PreprovisionSpecs []PreprovisionSpec `json:"preprovision,omitempty"`
}

type PreprovisionSpec struct {
	Driver string `json:"driver"`
	Region string `json:"region"`
	Plan   string `json:"plan"`
	Count  int    `json:"count"`
}

func loadConfig() (*Config, error) {
	path := os.Getenv("HOST_PROVISIONER_CONFIG_PATH")
	if path == "" {
		return nil, errors.New("provisioner: HOST_PROVISIONER_CONFIG_PATH is not defined")
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("provisioner: failed to read host provisioner config file at %s", path)
	}
	var hcfg Config
	if err := json.Unmarshal(data, &hcfg); err != nil {
		return nil, fmt.Errorf("provisioner: could not load host provisioner config file at %s: %s", path, err)
	}
	return &hcfg, nil
}
