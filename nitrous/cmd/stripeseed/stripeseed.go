package main

import (
	"os"

	"github.com/nitrous-io/nitrous-docker/vendor/src/github.com/Sirupsen/logrus"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/plan"
)

func init() {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		logrus.Fatal("STRIPE_SECRET_KEY environment variable not set")
	}
	stripe.Key = key
}

func main() {
	plans := []*stripe.PlanParams{
		{
			ID:       "free",
			Name:     "Starter",
			Currency: "USD",
			Amount:   0,
			Interval: stripe.PlanInterval("month"),
		},
		{
			ID:       "basic",
			Name:     "Basic",
			Currency: "USD",
			Amount:   900,
			Interval: stripe.PlanInterval("month"),
		},
		{
			ID:       "standard",
			Name:     "Standard",
			Currency: "USD",
			Amount:   2900,
			Interval: stripe.PlanInterval("month"),
		},
		{
			ID:       "plus",
			Name:     "Plus",
			Currency: "USD",
			Amount:   7900,
			Interval: stripe.PlanInterval("month"),
		},
	}

	for _, p := range plans {
		stripePlan, err := plan.New(p)
		if err != nil {
			logrus.Fatalf("Failed to create plan %#v in Stripe, err: %v", p, err)
		}

		logrus.Infof("Created new plan in stripe: %#v", stripePlan)
	}

	os.Exit(0)
}
