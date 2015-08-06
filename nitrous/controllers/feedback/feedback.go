package feedback

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
)

func Send(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	subject := r.FormValue("subject")
	body := r.FormValue("body")

	if subject == "" && body == "" {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{"error": "invalid_request"}
		return nil
	}

	userAgent := r.FormValue("userAgent")
	platform := r.FormValue("platform")
	currentURL := r.FormValue("currentURL")

	hosts, err := host.FindAllByUser(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	var hostsInfo []string
	for _, host := range hosts {
		plan, err := plan.FindByID(env.DB, host.PlanID)
		if err != nil {
			return err
		}
		subscription, err := subscription.Find(env.DB, host.SubscriptionID.Int64)
		if err != nil {
			return err
		}
		hostInfo := fmt.Sprintf("\n\tSlug: %s\n\tRegion Identifier: %s\n\tState: %v\n\tPlan Name: %s\n\tSubscription State: %s",
			&host.Slug,
			&host.RegionIdentifier.String,
			&host.State, &plan.Name,
			&subscription.State)

		hostsInfo = append(hostsInfo, hostInfo)
	}
	workspaceInfo := strings.Join(hostsInfo, "\n")

	billingProfile, err := env.User.BillingProfile(env.DB)
	if err != nil {
		return err
	}

	email := env.User.Email
	name := env.User.Name
	username := env.User.Username

	subject = fmt.Sprintf("[nitrous-pro] %s", subject)
	body = fmt.Sprintf("Username: %s\nName: %s\nEmail: %s\n%s", username, name, email, body)
	metadata := strings.Join([]string{
		fmt.Sprintf("User Agent: %s", userAgent),
		fmt.Sprintf("Platform: %s", platform),
		fmt.Sprintf("Current URL: %s", currentURL),
		fmt.Sprintf("User Info:\n\tID: %v\n\tUsername: %s\n\tEmail: %s\n\tConfirmed At: %v, Phone Authed: %t\n\tBilling Profile State: %s",
			env.User.ID,
			username,
			email,
			env.User.ConfirmedAt.Time,
			env.User.PhoneAuthed,
			billingProfile.State),
		fmt.Sprintf("Workspace(s) Info: %s", workspaceInfo),
	}, "\n")

	customer := helpscout.Person{
		Email:     email,
		FirstName: name,       // Optional
		Typ:       "customer", // Optional
	}
	mailbox := helpscout.MailboxRef{
		ID:   envvars.HelpScoutMailboxID,
		Name: envvars.HelpScoutMailboxName, // Optional
	}
	threads := []*helpscout.Thread{
		&helpscout.Thread{
			Typ: "customer",
			CreatedBy: helpscout.Person{
				Typ:   "customer",
				Email: email,
			},
			Body: body,
		},
		&helpscout.Thread{
			Typ: "note",
			CreatedBy: helpscout.Person{
				Typ: "user",
				ID:  envvars.HelpScoutUserID,
			},
			Body: metadata,
		},
	}

	err = helpscout.CreateConversation(customer, subject, mailbox, threads)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
	return nil
}

// TODO: Add tags field?
