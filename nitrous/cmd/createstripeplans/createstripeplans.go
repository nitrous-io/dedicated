package main

import (
	"os"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"
	splan "github.com/stripe/stripe-go/plan"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		panic("STRIPE_SECRET_KEY environment variable not set")
	}
}

func main() {
	dbURL := os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		logr.FatalExit("NITROUS_PG_URL is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	if err := dbConn.MigrateAll(); err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	plans, err := plan.All(dbConn)
	if err != nil {
		logr.FatalExit(err)
	}

	for _, p := range plans {
		params := &stripe.PlanParams{
			ID:       p.Slug,
			Currency: "usd",
			Amount:   uint64(p.Price),
			Interval: "month",
			Name:     p.Name,
		}
		_, err := splan.New(params)
		if err != nil {
			logr.FatalExit(err)
		}
	}
}
