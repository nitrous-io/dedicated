package container

import (
	"database/sql"
	"log"
	"testing"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

var dbStructure = []string{
	`DROP TABLE IF EXISTS containers;`,
	`DROP TABLE IF EXISTS hosts;`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		ip_address character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
	`CREATE TABLE containers (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		host_id bigint NOT NULL);`,
	`CREATE UNIQUE INDEX index_containers_on_slug ON containers USING btree (slug);`,
}

var fixtures = []*Container{
	{Slug: "daniel-box", HostIP: "10.0.0.1"},
	{Slug: "md-box", HostIP: "10.0.0.2"},
	{Slug: "dqminh-box", HostIP: "10.0.0.3"},
}

func testNavigator(domain string) n.Navigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
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
	for _, b := range containers {
		var hostID int
		var err error
		err = db.QueryRow(`INSERT INTO hosts (slug, ip_address) VALUES ($1, $2) RETURNING ID`,
			b.HostIP, b.HostIP).Scan(&hostID)
		if err != nil {
			return err
		}
		err = db.QueryRow(`INSERT INTO containers (slug, host_id) VALUES ($1, $2) RETURNING ID`,
			b.Slug, hostID).Scan(&b.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestFind(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	c, err := n.find(fixtures[0].Slug)
	if err != nil || c.Slug != fixtures[0].Slug || c.HostIP != fixtures[0].HostIP {
		t.Fatal("Failed to find a container with valid slug")
	}
	_, err = n.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find a container with invalid slug")
	}
}

func TestLookup(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	c := fixtures[0]
	r, err := n.Lookup(c.Slug + ".nitrousbox.com.")
	if err != nil || r[0].(*dns.A).A.String() != c.HostIP {
		t.Errorf("Failed to get correct dns record, got %v, expected DNS A with %s", r[0], c.HostIP)
	}
	_, err = n.Lookup("invalid-box.nitrousbox.com.")
	if err == nil {
		t.Error("should not be able to look up invalid box")
	}
	_, err = n.Lookup("daniel-box.nitrous.io.")
	if err == nil {
		t.Error("should not be able to look up invalid domain")
	}
}
