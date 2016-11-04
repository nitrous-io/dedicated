package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("failed with error: %v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func run() error {
	dbURL := os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		log.Fatal("NITROUS_PG_URL is required.")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database, err: %v", err)
	}

	filepath := os.Getenv("FILE")
	f, err := os.Open(filepath)
	if err != nil {
		log.Fatalf("error reading file %q, err: %v", filepath, err)
	}

	csvRdr := csv.NewReader(f)
	csvRdr.FieldsPerRecord = 7

	// Manually read past the first line which contains the header.
	if _, err := csvRdr.Read(); err != nil {
		log.Printf("error reading first CSV record: %v", err)
	}

	for {
		record, err := csvRdr.Read()
		if err != nil {
			log.Printf("error reading next CSV record: %v", err)
			break
		}

		// email, username, container slug, container name, URL, timestamp of backup, host ID
		slug := record[2]

		cn := &container.Container{}
		if err := dbConn.Get(cn,
			`SELECT * FROM containers WHERE slug = $1 AND containers.terminated_at IS NULL;`, slug); err != nil {
			log.Printf("error querying container %q from db: %v", slug, err)
		}

		fmt.Printf("%s,%s\n", strings.Join(record, ","), cn.Image)
	}

	return nil
}
