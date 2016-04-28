package geoip

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fiorix/freegeoip"
)

// URL to download MaxMind's GeoLite2 database. See http://dev.maxmind.com/geoip/geoip2/geolite2/.
// Downloading on-the-fly is not ideal but is much less work since no infra is
// required. The github.com/fiorix/freegeoip conveniently does the downloading
// and regular updating of the database.
const maxMindCountryURL = "http://geolite.maxmind.com/download/geoip/database/GeoLite2-Country.mmdb.gz"

var (
	sharedDB *freegeoip.DB
	once     sync.Once

	UpdateInterval   = 24 * time.Hour
	MaxRetryInterval = time.Hour

	downloadTimeout = 5 * time.Second
)

type Locatable interface {
	LookupCountry(ip string) (countryCode string, err error)
}

type DB struct {
	db *freegeoip.DB
}

func New() (*DB, error) {
	if _, err := openDB(); err != nil {
		return nil, err
	}

	return &DB{db: sharedDB}, nil
}

// LookupCountry returns the ISO 3166-1 2-letter country code of an IP address.
func (d *DB) LookupCountry(ip string) (string, error) {
	var dq freegeoip.DefaultQuery
	if err := d.db.Lookup(net.ParseIP(ip), &dq); err != nil {
		return "", fmt.Errorf("failed to lookup geographical location of IP address %q, err: %v", ip, err)
	}

	return dq.Country.ISOCode, nil
}

func openDB() (*freegeoip.DB, error) {
	var err error
	once.Do(func() {
		sharedDB, err = freegeoip.OpenURL(maxMindCountryURL, UpdateInterval, MaxRetryInterval)
		if err == nil {
			select {
			case <-sharedDB.NotifyOpen():
				// Wait until DB has been downloaded and is ready.
			case err := <-sharedDB.NotifyError():
				sharedDB = nil
				err = fmt.Errorf("failed to download MaxMind database, err: %v", err)
			case <-time.After(downloadTimeout):
				err = fmt.Errorf("timed out while downloading MaxMind database")
			}
		}
	})

	return sharedDB, err
}
