package container

import (
	"database/sql"
	"fmt"
	"log"
	"testing"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

var dbStructure = []string{
	`DROP TABLE IF EXISTS containers;`,
	`DROP TABLE IF EXISTS hosts;`,
	`DROP TABLE IF EXISTS regions;`,
	`CREATE TABLE regions (
		id int PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);`,
	`INSERT INTO regions (id, slug) VALUES (1, 'sgp');`,
	`INSERT INTO regions (id, slug) VALUES (2, 'nyc');`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		region_id int NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
	`CREATE TABLE containers (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		host_id bigint NOT NULL);`,
	`CREATE UNIQUE INDEX index_containers_on_slug ON containers USING btree (slug);`,
}

var fixtures = []*Container{
	{Slug: "daniel-box", RegionID: 1, RegionSlug: "sgp"},
	{Slug: "md-box", RegionID: 1, RegionSlug: "sgp"},
	{Slug: "dqminh-box", RegionID: 2, RegionSlug: "nyc"},
}

func testNavigator(domain string) n.Navigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
		"sgp":          "exampleproxy-sgp.com",
	}
	b, err := New(domain, config)
	if err != nil {
		log.Println("failed to create navigator", err)
	}
	for _, s := range dbStructure {
		_, err = b.(*Navigator).db.Exec(s)
		if err != nil {
			log.Println("failed to set db structure", err, s)
		}
	}
	if err := insertContainer(b.(*Navigator).db, fixtures); err != nil {
		log.Println("failed to set db fixtures", err)
	}
	return b
}

func insertContainer(db *sql.DB, containers []*Container) error {
	for i, c := range containers {
		var hostID int
		var err error
		err = db.QueryRow(`INSERT INTO hosts (slug, region_id) VALUES ($1, $2) RETURNING ID`,
			fmt.Sprintf("host-%d", i), c.RegionID).Scan(&hostID)
		if err != nil {
			return err
		}
		err = db.QueryRow(`INSERT INTO containers (slug, host_id) VALUES ($1, $2) RETURNING ID`,
			c.Slug, hostID).Scan(&c.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestName(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	if n.Name() != "container" {
		t.Fatal("a navigator should have a name.")
	}
}

func TestFind(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	c, err := n.find(fixtures[0].Slug)
	if err != nil || c.Slug != fixtures[0].Slug || c.RegionID != fixtures[0].RegionID ||
		c.RegionSlug != fixtures[0].RegionSlug {
		t.Fatal("Failed to find a container with valid slug")
	}
	_, err = n.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find a container with invalid slug")
	}
}

func TestLookup(t *testing.T) {
	nav := testNavigator("nitrousbox.com.").(*Navigator)
	f := fixtures[0]
	r, err := nav.Lookup(f.Slug + ".nitrousbox.com.")
	cname := fmt.Sprintf("%s.%s.", f.Slug, nav.config[f.RegionSlug])
	if err != nil || r[0].(*dns.CNAME).Target != cname {
		t.Errorf("unexpected dns record, got %v, expected DNS CNAME: %s", r[0], cname)
	}
	_, err = nav.Lookup("invalid-box.nitrousbox.com.")
	if err == nil {
		t.Error("should not be able to look up invalid box")
	}
	_, err = nav.Lookup("daniel-box.nitrous.io.")
	if err == nil {
		t.Error("should not be able to look up invalid domain")
	}
}
