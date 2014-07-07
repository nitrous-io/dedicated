package command

import (
	"os"
	"path/filepath"
)

// an Environment is meant to be used as a singleton that stores env vars --
// call ConfigureEnvironment() to populate it accordingly.
type Environment struct {
	APIURL     string
	ConfigPath string
}

// defaults for missing env vars
const (
	defaultAPIURL = "http://127.0.0.1:3000"
)

func ConfigureEnvironment() *Environment {
	e := &Environment{}

	e.ConfigPath = os.Getenv("NITRO_CONFIG_PATH")
	if e.ConfigPath == "" {
		e.ConfigPath = defaultConfigPath()
	}

	e.APIURL = os.Getenv("NITRO_API_URL")
	if e.APIURL == "" {
		e.APIURL = defaultAPIURL
	}

	return e
}

func defaultConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".nitrocfg")
}
