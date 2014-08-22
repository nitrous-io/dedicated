package command

import (
	"fmt"
	"os"
	"path/filepath"
)

// an Environment is meant to be used as a singleton that stores env vars --
// call ConfigureEnvironment() to populate it accordingly.
type Environment struct {
	APIURL     string
	ConfigPath string
	Config     *ConfigFile
	AuthToken  string
}

// defaults for missing env vars
const (
	DefaultAPIURL = "http://127.0.0.1:3333"
)

func ConfigureEnvironment() *Environment {
	e := &Environment{}

	e.ConfigPath = os.Getenv("NITRO_CONFIG_PATH")
	if e.ConfigPath == "" {
		e.ConfigPath = defaultConfigPath()
	}

	e.APIURL = os.Getenv("NITRO_API_URL")
	if e.APIURL == "" {
		e.APIURL = DefaultAPIURL
	}

	config, err := loadConfig(e.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nitro: unexpected error: %v\n", err)
		os.Exit(1)
	}
	e.Config = config

	if authConfig, ok := config.Auths[e.APIURL]; ok {
		e.AuthToken = authConfig.Token
	}

	return e
}

func defaultConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".nitrocfg")
}
