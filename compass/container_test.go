package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

var dbStructureContainer = []string{
	`DROP TABLE IF EXISTS containers;`,
	`DROP TABLE IF EXISTS hosts;`,
	`DROP TABLE IF EXISTS regions;`,
	`DROP TABLE IF EXISTS provider_templates;`,
	`CREATE TABLE regions (
		id int PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);`,
	`INSERT INTO regions (id, slug) VALUES (1, 'sgp');`,
	`INSERT INTO regions (id, slug) VALUES (2, 'nyc');`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		region_id int NOT NULL,
		provider_template_id int NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
	`CREATE TABLE provider_templates (
		id bigserial PRIMARY KEY NOT NULL,
		provider character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_provider_templates_on_provider ON provider_templates USING btree (provider);`,
	`INSERT INTO provider_templates (id, provider) VALUES (1, 'digitalocean');`,
	`INSERT INTO provider_templates (id, provider) VALUES (2, 'amazon');`,
	`CREATE TABLE containers (
		id bigserial PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL,
		host_id bigint NOT NULL);`,
	`CREATE UNIQUE INDEX index_containers_on_slug ON containers USING btree (slug);`,
}

type fixtureContainer struct {
	Container
	providerTemplateID int64
}

var fixturesContainer = []fixtureContainer{
	{Container: Container{Slug: "daniel-box", RegionID: 1, RegionSlug: "sgp"}, providerTemplateID: 2},
	{Container: Container{Slug: "md-box", RegionID: 1, RegionSlug: "sgp"}, providerTemplateID: 1},
	{Container: Container{Slug: "dqminh-box", RegionID: 2, RegionSlug: "nyc"}, providerTemplateID: 2},
}

func testContainerNavigator(domain string) *ContainerNavigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
		"sgp":          "exampleproxy-sgp.com",
	}
	b, err := NewContainerNavigator(domain, config)
	if err != nil {
		log.Println("failed to create navigator", err)
	}
	for _, s := range dbStructureContainer {
		if _, err := b.db.Exec(s); err != nil {
			log.Println("failed to set db structure", err, s)
		}
	}
	if err := insertContainers(b.db, fixturesContainer); err != nil {
		log.Println("failed to set db fixtures", err)
	}
	return b
}

func insertContainers(db *sql.DB, fixtures []fixtureContainer) error {
	for i, f := range fixtures {
		var hostID int
		var err error
		err = db.QueryRow(`INSERT INTO hosts (slug, region_id, provider_template_id) VALUES ($1, $2, $3) RETURNING ID`,
			fmt.Sprintf("host-%d", i), f.RegionID, f.providerTemplateID).Scan(&hostID)
		if err != nil {
			return err
		}
		err = db.QueryRow(`INSERT INTO containers (slug, host_id) VALUES ($1, $2) RETURNING ID`,
			f.Slug, hostID).Scan(&f.Container.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestContainerName(t *testing.T) {
	n := testContainerNavigator("nitrousbox.com.")
	if n.Name() != "container" {
		t.Fatal("a navigator should have a name.")
	}
}

func TestContainerFind(t *testing.T) {
	n := testContainerNavigator("nitrousbox.com.")
	c, err := n.find(fixturesContainer[0].Slug)
	if err != nil || c.Slug != fixturesContainer[0].Slug || c.RegionID != fixturesContainer[0].RegionID ||
		c.RegionSlug != fixturesContainer[0].RegionSlug {
		t.Fatal("Failed to find a container with valid slug")
	}
	_, err = n.find("invalid-box")
	if err == nil {
		t.Fatal("should not be able to find a container with invalid slug")
	}
}

func TestContainerLookup(t *testing.T) {
	nav := testContainerNavigator("nitrousbox.com.")
	f := fixturesContainer[0]
	h, err := nav.find(f.Slug)
	r, err := nav.Lookup(f.Slug + ".nitrousbox.com.")
	cname := fmt.Sprintf("%s.%s.", h.Provider, nav.config[f.RegionSlug])
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

func TestContainerLookupCaseInsensitive(t *testing.T) {
	nav := testContainerNavigator("nitrousbox.com.")
	f := fixturesContainer[0]
	h, err := nav.find(f.Slug)
	slug := strings.ToUpper(f.Slug)
	r, err := nav.Lookup(slug + ".nitrousbox.com.")
	if err != nil {
		t.Fatalf("Unexpected lookup error: %v", err)
	}
	cname := fmt.Sprintf("%s.%s.", h.Provider, nav.config[f.RegionSlug])
	if r[0].(*dns.CNAME).Target != cname {
		t.Fatalf("Expected CNAME %s but got %v", cname, r[0])
	}
}
