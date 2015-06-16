package host

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

var dbStructure = []string{
	`DROP TABLE IF EXISTS hosts;`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		ip_address character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
}

var fixtures = []*Host{
	{Slug: "daniel-box", IpAddress: "10.0.0.1"},
	{Slug: "md-box", IpAddress: "10.0.0.2"},
	{Slug: "dqminh-box", IpAddress: "10.0.0.3"},
}

func testNavigator(domain string) n.Navigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
	}
	b, err := New(domain, config)
	if err != nil {
		logrus.Error("failed to create boxhost", err)
	}
	for _, s := range dbStructure {
		_, err = b.(*Navigator).db.Exec(s)
		if err != nil {
			logrus.Error("failed to set db structure", err, s)
		}
	}
	if err := insertHost(b.(*Navigator).db, fixtures); err != nil {
		logrus.Error("failed to set db fixtures", err)
	}
	return b
}

func insertHost(db *sql.DB, values []*Host) error {
	stm, err := db.Prepare(`INSERT INTO hosts (slug, ip_address)
		VALUES ($1, $2) RETURNING ID`)
	if err != nil {
		return fmt.Errorf("stm error %v", err)
	}
	for _, b := range values {
		err := stm.QueryRow(b.Slug, b.IpAddress).Scan(&b.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestName(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	if n.Name() != "host" {
		t.Fatal("a navigator should have a name.")
	}
}

func TestFind(t *testing.T) {
	nav := testNavigator("example.com.").(*Navigator)
	bh := fixtures[0]
	host, err := nav.find(bh.Slug)
	if err != nil || host.Slug != "daniel-box" || host.IpAddress != "10.0.0.1" {
		t.Fatal("Failed to find host")
	}
	_, err = nav.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find invalid box")
	}
}

func TestLookup(t *testing.T) {
	nav := testNavigator("example.com.").(*Navigator)
	h := fixtures[0]
	r, err := nav.Lookup(fmt.Sprintf("%s.example.com", h.Slug))
	if err != nil || r[0].(*dns.A).A.String() != h.IpAddress {
		t.Fatal("Failed to get correct dns record")
	}
	_, err = nav.Lookup("invalid-box.example.com.")
	if err == nil {
		t.Fatal("should not be able to look up invalid box")
	}
	_, err = nav.Lookup(fmt.Sprintf("%s.nitrous.io", h.Slug))
	if err == nil {
		t.Fatal("should not be able to look up invalid domain")
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	nav := testNavigator("example.com.").(*Navigator)
	r, err := nav.Lookup("Daniel-BOX.Example.Com")
	if err != nil {
		t.Fatalf("Unexpected lookup error: %v", err)
	}
	if len(r) == 0 {
		t.Fatal("Expected answers but got none")
	}
	if r[0].(*dns.A).A.String() != "10.0.0.1" {
		t.Fatalf("Expected first answer to be a valid A record but got %v", r[0])
	}
}
