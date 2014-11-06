package docker

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type KeyManager struct {
	path string
}

func NewKeyManager(path string) *KeyManager {
	return &KeyManager{path: path}
}

func (km *KeyManager) Read() ([]string, error) {
	bkeys, err := ioutil.ReadFile(km.path)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for _, key := range strings.Split(strings.TrimSpace(string(bkeys)), "\n") {
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (km *KeyManager) Write(keys []string) error {
	dir := path.Dir(km.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.Create(km.path)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, key := range keys {
		key = strings.TrimSpace(strings.Replace(key, "\n", "", -1)) + "\n"
		_, err = file.Write([]byte(key))
		if err != nil {
			return err
		}
	}
	return nil
}
