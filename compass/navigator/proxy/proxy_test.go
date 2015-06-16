package proxy

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

var dbStructure = []string{
	`DROP TABLE IF EXISTS hosts;`,
	`CREATE TABLE hosts (
		id bigserial PRIMARY KEY NOT NULL,
		region_id int NOT NULL,
		provider_template_id int NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_hosts_on_slug ON hosts USING btree (slug);`,
	`DROP TABLE IF EXISTS regions;`,
	`CREATE TABLE regions (
		id int PRIMARY KEY NOT NULL,
		slug character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_regions_on_slug ON regions USING btree (slug);`,
	`DROP TABLE IF EXISTS provider_templates;`,
	`CREATE TABLE provider_templates (
		id bigserial PRIMARY KEY NOT NULL,
		provider character varying(255) NOT NULL);`,
	`CREATE UNIQUE INDEX index_provider_templates_on_provider ON provider_templates USING btree (provider);`,
	`INSERT INTO provider_templates (id, provider) VALUES (1, 'digitalocean');`,
	`INSERT INTO provider_templates (id, provider) VALUES (2, 'amazon');`,
	`INSERT INTO regions (id, slug) VALUES (1, 'sgp');`,
	`INSERT INTO regions (id, slug) VALUES (2, 'nyc');`,
}

var fixtures = []*Host{
	{Slug: "daniel-box", RegionID: 1, ProviderID: 2},
	{Slug: "md-box", RegionID: 1, ProviderID: 1},
	{Slug: "dqminh-box", RegionID: 2, ProviderID: 2},
}

func testNavigator(domain string) n.Navigator {
	config := map[string]string{
		"database_url": "host=/var/run/postgresql dbname=compass_test sslmode=disable",
		"sgp":          "exampleproxy-sgp.com",
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
	stm, err := db.Prepare(`INSERT INTO hosts (slug, region_id, provider_template_id)
		VALUES ($1, $2, $3) RETURNING ID`)
	if err != nil {
		return fmt.Errorf("stm error %v", err)
	}
	for _, b := range values {
		err := stm.QueryRow(b.Slug, b.RegionID, b.ProviderID).Scan(&b.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestName(t *testing.T) {
	n := testNavigator("nitrousbox.com.").(*Navigator)
	if n.Name() != "proxy" {
		t.Fatal("a navigator should have a name.")
	}
}

func TestFind(t *testing.T) {
	nav := testNavigator("exampleproxy.com.").(*Navigator)
	bh := fixtures[0]
	host, err := nav.find(bh.Slug)
	if err != nil || host.Slug != "daniel-box" || host.RegionID != 1 || host.RegionSlug != "sgp" || host.Provider != "amazon" {
		t.Fatalf("Host(%#v), expected daniel-box with region sgp and provider amazon", host)
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
	if err != nil {
		t.Fatalf("Failed to find host, err: %v", err)
	}
	cname := fmt.Sprintf("%s.%s.", host.Provider, nav.config[host.RegionSlug])
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

func TestLookupCaseInsensitive(t *testing.T) {
	nav := testNavigator("exampleproxy.com.").(*Navigator)
	h := fixtures[1]
	host, err := nav.find(h.Slug)
	if err != nil {
		t.Fatalf("Failed to find host, err: %v", err)
	}
	r, err := nav.Lookup(fmt.Sprintf("%s.exampleproxy.com", strings.ToUpper(h.Slug)))
	if err != nil {
		t.Fatalf("Unexpected lookup error: %v", err)
	}
	cname := fmt.Sprintf("%s.%s.", host.Provider, nav.config[host.RegionSlug])
	if r[0].(*dns.CNAME).Target != cname {
		t.Fatalf("Expected CNAME %s but got %v", cname, r[0])
	}
}
