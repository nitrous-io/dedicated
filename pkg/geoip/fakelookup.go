package geoip

import "errors"

type FakeDB struct {
	M map[string]string
}

func (f *FakeDB) LookupCountry(ip string) (string, error) {
	if code, ok := f.M[ip]; ok {
		return code, nil
	}

	return "", errors.New("failed to lookup IP")
}
