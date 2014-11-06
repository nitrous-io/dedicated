package docker

import (
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/util"
)

// a KeyManager supports reading/writing keys from/to an authorized_keys file
// found at path
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

// Add reads the existing keys at path and appends newKeys to them, removing
// duplicates
func (km *KeyManager) Add(newKeys []string) error {
	existingKeys, err := km.Read()
	if err != nil {
		return err
	}

	// validate pubkeys and remove duplicates
	keymap, err := keysToKeyMap(append(newKeys, existingKeys...))
	if err != nil {
		return err
	}
	return km.Write(keyMapToKeys(keymap))
}

// Clear removes all keys at path. Note: it will create the file at path if not
// already present.
func (km *KeyManager) Clear() error {
	return km.Write([]string{})
}

// Remove deletes a key given its fingerprint. It will not error if the
// fingerprint given does not exist.
func (km *KeyManager) Remove(fingerprint string) error {
	keys, err := km.Read()
	if err != nil {
		return err
	}

	keymap, err := keysToKeyMap(keys)
	if err != nil {
		return err
	}
	delete(keymap, fingerprint)
	return km.Write(keyMapToKeys(keymap))
}

// converts []pubkey to map[keyFingerprint]pubkey
func keysToKeyMap(keys []string) (map[string]string, error) {
	keymap := map[string]string{}
	for _, key := range keys {
		fingerprint, err := util.Md5Fingerprint(key)
		if err != nil {
			return nil, err
		}
		keymap[fingerprint] = key
	}
	return keymap, nil
}

// converts map[keyFingerprint]pubkey back to []pubkey
func keyMapToKeys(keymap map[string]string) []string {
	keys := make([]string, len(keymap))
	i := 0
	for _, key := range keymap {
		keys[i] = key
		i++
	}
	return keys
}
