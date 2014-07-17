package command

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type ConfigFile struct {
	Auths       map[string]AuthConfig `json:"auths,omitempty"`
	DefaultHost string                `json:"default_host,omitempty"`
}

type AuthConfig struct {
	Username string `json:"username"`
	Token    string `json:"auth"`
}

func LoadConfig() (*ConfigFile, error) {
	return loadConfig(Env.ConfigPath)
}

func loadConfig(path string) (*ConfigFile, error) {
	configFile := ConfigFile{Auths: make(map[string]AuthConfig)}
	fd, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &configFile, nil
		}
		return nil, err
	}
	defer fd.Close()

	b, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return &configFile, nil
	}

	err = json.Unmarshal(b, &configFile)
	if err != nil {
		return nil, err
	}

	return &configFile, nil
}

func SaveConfig(configFile *ConfigFile) error {
	b, err := json.Marshal(&configFile)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(Env.ConfigPath, b, 0600)
}

func DefaultHost() string {
	config, err := LoadConfig()
	if err != nil {
		return ""
	}
	return config.DefaultHost
}

func SaveDefaultHost(h string) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	config.DefaultHost = h
	return SaveConfig(config)
}
