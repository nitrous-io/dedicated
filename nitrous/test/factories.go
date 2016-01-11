package test

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type HostStack struct {
	R    *region.Region
	P    *plan.Plan
	PT   *providertemplate.ProviderTemplate
	H    *host.Host
	U    *user.User
	O    *org.Org
	S    *subscription.Subscription
	Cert *cert.Cert
}

var (
	// track number of hoststacks successfully created so we can generate unique
	// fields
	HostStackCount int

	// provider templates will be created with this provider, set via env
	// DEFAULT_PROVIDER
	defaultProvider string
)

func init() {
	defaultProvider = envvars.DefaultProvider
	if defaultProvider == "" {
		fmt.Fprintln(os.Stderr, "WARN: DEFAULT_PROVIDER is not set, using 'docker'")
		defaultProvider = "docker"
	}
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

	makeUnique := func(val string) string {
		if HostStackCount == 0 {
			return val
		}
		return fmt.Sprintf("%s%d", val, HostStackCount)
	}

	if hs.U == nil {
		var username string
		if HostStackCount == 0 {
			username = "hoststackuser"
		} else {
			username = makeUnique("hoststackuser")
		}
		hs.U = &user.User{
			Username: username,
			Email:    fmt.Sprintf("%s@example.com", username),
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
		hs.R = &region.Region{Slug: makeUnique("apse"), Name: "Southeast Asia"}
	}

	if hs.R.ID == 0 {
		err = hs.R.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.P == nil {
		hs.P = &plan.Plan{
			Slug:             makeUnique("plan"),
			Name:             makeUnique("desc"),
			MaxContainers:    1,
			MaxCollaborators: sql.NullInt64{1, true},
			Price:            500,
			Storage:          "4 GB",
			Memory:           "512 MB",
		}
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
			Name:     "default-1",
			Provider: defaultProvider,
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

	if hs.S != nil && hs.S.ID == 0 {
		hs.S.PlanID = hs.P.ID
		if hs.S.State == "" {
			hs.S.State = "active"
		}
		if hs.S.BillingProfileID == 0 {
			bp, err := hs.U.BillingProfile(tx)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
			hs.S.BillingProfileID = bp.ID
		}
		err = hs.S.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.H == nil {
		hs.H = &host.Host{}
	}
	if hs.H.Slug == "" {
		hs.H.Slug = makeUnique("hoststack")
	}
	if hs.H.State == "" {
		hs.H.State = host.StateRunning
	}

	if !hs.H.ProviderIdentifier.Valid {
		hs.H.ProviderIdentifier = sql.NullString{"host-0001", true}
	}

	hs.H.RegionID = hs.R.ID
	hs.H.PlanID = hs.P.ID
	hs.H.ProviderTemplateID = hs.PT.ID
	hs.H.UserID = hs.U.ID

	if hs.O != nil {
		hs.H.OrgID = sql.NullInt64{hs.O.ID, true}
	}

	err = hs.H.Insert(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// create a certificate for the host.  This
	// certificate will not work for TLS access to the
	// host (via freighterclient), use
	// GenerateHostCert(db, hs) to produce a usable cert
	crt := &cert.Cert{
		ServerCert: localhostCert,
		ServerKey:  localhostKey,
	}
	if err := crt.Insert(tx, hs.H.ID); err != nil {
		tx.Rollback()
		return nil, err
	}
	hs.Cert = crt

	if hs.S != nil {
		if err := hs.H.UpdateColumn(tx, "subscription_id", sql.NullInt64{hs.S.ID, true}); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	HostStackCount += 1

	return hs, nil
}
