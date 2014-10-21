package proxy

import (
	"database/sql"
	"fmt"
	"log"
	"testing"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

var dbStructure = []string{
	`DROP TABLE IF EXISTS hosts;`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		region_id int NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
	`DROP TABLE IF EXISTS regions;`,
	`CREATE TABLE regions (
		id int PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);`,
	`INSERT INTO regions (id, slug) VALUES (1, 'sgp');`,
	`INSERT INTO regions (id, slug) VALUES (2, 'nyc');`,
}

var fixtures = []*Host{
	{Slug: "daniel-box", RegionID: 1},
	{Slug: "md-box", RegionID: 1},
	{Slug: "dqminh-box", RegionID: 2},
}

func testNavigator(domain string) n.Navigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
		"sgp":          "exampleproxy-sgp.com",
	}
	b, err := New(domain, config)
	if err != nil {
		log.Println("failed to create boxhost", err)
	}
	for _, s := range dbStructure {
		_, err = b.(*Navigator).db.Exec(s)
		if err != nil {
			log.Println("failed to set db structure", err, s)
		}
	}
	if err := insertHost(b.(*Navigator).db, fixtures); err != nil {
		log.Println("failed to set db fixtures", err)
	}
	return b
}

func insertHost(db *sql.DB, values []*Host) error {
	stm, err := db.Prepare(`INSERT INTO hosts (slug, region_id)
		VALUES ($1, $2) RETURNING ID`)
	if err != nil {
		return fmt.Errorf("stm error %v", err)
	}
	for _, b := range values {
		err := stm.QueryRow(b.Slug, b.RegionID).Scan(&b.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestFind(t *testing.T) {
	nav := testNavigator("exampleproxy.com.").(*Navigator)
	bh := fixtures[0]
	host, err := nav.find(bh.Slug)
	if err != nil || host.Slug != "daniel-box" || host.RegionID != 1 || host.RegionSlug != "sgp" {
		t.Fatalf("Host(%#v), expected daniel-box with region sgp", host)
	}
	_, err = nav.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find invalid box")
	}
}

func TestLookup(t *testing.T) {
	nav := testNavigator("exampleproxy.com.").(*Navigator)
	h := fixtures[1]
	host, err := nav.find(h.Slug)
	cname := fmt.Sprintf("%s.%s.", h.Slug, nav.config[host.RegionSlug])
	r, err := nav.Lookup(fmt.Sprintf("%s.exampleproxy.com", h.Slug))

	if err != nil || r[0].(*dns.CNAME).Target != cname {
		t.Fatal("Failed to get correct dns record")
	}

	_, err = nav.Lookup("invalid-box.exampleproxy.com.")
	if err == nil {
		t.Fatal("should not be able to look up invalid box")
	}

	_, err = nav.Lookup(fmt.Sprintf("%s.nitrous.io", h.Slug))
	if err == nil {
		t.Fatal("should not be able to look up invalid domain")
	}
}
