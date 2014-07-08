package main

import (
	"database/sql"
	"os"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
)

const DBEnvVar = "NITROUS_PG_URL"

func seed(d *db.Database, logr *logger.Logger) {
	var err error
	var i int

	err = d.QueryRow("SELECT 1 FROM regions WHERE id IS NOT NULL LIMIT 1").Scan(&i)
	if err == sql.ErrNoRows {
		regions := []region.Region{
			region.Region{Name: "apse", Description: "Southeast Asia"},
			region.Region{Name: "use", Description: "US East"},
			region.Region{Name: "usw", Description: "US West"},
			region.Region{Name: "euw", Description: "Europe"},
		}
		for _, r := range regions {
			err := r.Insert(d)
			if err != nil {
				logr.Infof("Failed to add %v - %v", r.Name, err)
			}
			logr.Infof("Added region %v", r.Name)
		}
	}

	err = d.QueryRow("SELECT 1 FROM plans WHERE id IS NOT NULL LIMIT 1").Scan(&i)
	if err == sql.ErrNoRows {
		plans := []plan.Plan{
			plan.Plan{Name: "nano", MaxContainers: 1, Price: 500},
			plan.Plan{Name: "micro", MaxContainers: 2, Price: 1000},
			plan.Plan{Name: "small", MaxContainers: 3, Price: 1500},
			plan.Plan{Name: "medium", MaxContainers: 4, Price: 2000},
			plan.Plan{Name: "large", MaxContainers: 5, Price: 2500},
		}
		for _, p := range plans {
			err := p.Insert(d)
			if err != nil {
				logr.Infof("Failed to add %v - %v", p.Name, err)
			}
			logr.Infof("Added plan %v", p.Name)
		}
	}
}

func main() {
	logr := &logger.Logger{Level: logger.LogLevelDebug}

	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logr.FatalExit(DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	err = dbConn.MigrateAll()
	if err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	seed(dbConn, logr)
}
