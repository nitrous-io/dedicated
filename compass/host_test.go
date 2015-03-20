package main

import (
	"database/sql"
	"fmt"
	"log"
	"testing"

	"github.com/miekg/dns"
)

var dbStructureHost = []string{
	`DROP TABLE IF EXISTS hosts;`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		ip_address character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
}

var fixturesHost = []*Host{
	{Slug: "daniel-box", IpAddress: "10.0.0.1"},
	{Slug: "md-box", IpAddress: "10.0.0.2"},
	{Slug: "dqminh-box", IpAddress: "10.0.0.3"},
}

func testHostNavigator(domain string) *HostNavigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
	}
	b, err := NewHostNavigator(domain, config)
	if err != nil {
		log.Println("failed to create boxhost", err)
	}
	for _, s := range dbStructureHost {
		_, err = b.db.Exec(s)
		if err != nil {
			log.Println("failed to set db structure", err, s)
		}
	}
	if err := insertHost(b.db, fixturesHost); err != nil {
		log.Println("failed to set db fixtures", err)
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

func TestHostName(t *testing.T) {
	n := testHostNavigator("nitrousbox.com.")
	if n.Name() != "host" {
		t.Fatal("a navigator should have a name.")
	}
}

func TestHostFind(t *testing.T) {
	nav := testHostNavigator("example.com.")
	bh := fixturesHost[0]
	host, err := nav.find(bh.Slug)
	if err != nil || host.Slug != "daniel-box" || host.IpAddress != "10.0.0.1" {
		t.Fatal("Failed to find host")
	}
	_, err = nav.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find invalid box")
	}
}

func TestHostLookup(t *testing.T) {
	nav := testHostNavigator("example.com.")
	h := fixturesHost[0]
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

func TestHostLookupCaseInsensitive(t *testing.T) {
	nav := testHostNavigator("example.com.")
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
