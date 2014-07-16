package test

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/nitrous-io/nitro/command"
)

func WriteConfig(conf *command.ConfigFile, path string) (string, error) {
	var f *os.File
	var err error
	if path == "" {
		f, err = ioutil.TempFile("", "nitrocfg")
	} else {
		f, err = os.Create(path)
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	os.Setenv("NITRO_CONFIG_PATH", f.Name())
	err = json.NewEncoder(f).Encode(conf)
	if err != nil {
		return "", err
	}
	command.Env = command.ConfigureEnvironment()
	return f.Name(), nil
}
