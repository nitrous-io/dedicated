package test

import (
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
)

type HostStack struct {
	R  *region.Region
	P  *plan.Plan
	PT *providertemplate.ProviderTemplate
	H  *host.Host
	U  *user.User
}

// CreateHostStack returns a HostStack with created user,
// region, plan, providertemplate, and host.
// If any model fields in hs are present, they are used, otherwise
// placeholder ones are created.
// If any inserts fail, the entire transaction is rolled back.
func CreateHostStack(db *db.Database, hs *HostStack) (*HostStack, error) {
	tx, err := db.Beginx()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if hs == nil {
		hs = &HostStack{}
	}

	if hs.U == nil {
		hs.U = &user.User{
			Username: "hoststackuser",
			Email:    "hoststackuser@example.com",
			Password: "123456",
		}
	}
	if hs.U.ID == 0 {
		err = hs.U.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.R == nil {
		hs.R = &region.Region{Name: "apse1", Description: "Southeast Asia"}
	}

	if hs.R.ID == 0 {
		err = hs.R.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.P == nil {
		hs.P = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
	}
	if hs.P.ID == 0 {
		err = hs.P.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.PT == nil {
		hs.PT = &providertemplate.ProviderTemplate{
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
	}
	hs.PT.RegionID = hs.R.ID
	hs.PT.PlanID = hs.P.ID

	if hs.PT.ID == 0 {
		err = hs.PT.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.H == nil {
		hs.H = &host.Host{
			Slug: "hoststack-1",
		}
	}
	if hs.H.State == "" {
		hs.H.State = host.StateRunning
	}

	hs.H.RegionID = hs.R.ID
	hs.H.PlanID = hs.P.ID
	hs.H.ProviderTemplateID = hs.PT.ID
	hs.H.UserID = hs.U.ID

	err = hs.H.Insert(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return hs, nil
}
