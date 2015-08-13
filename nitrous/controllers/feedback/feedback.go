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

var HelpScout helpscout.HelpScout

func init() {
	HelpScout = &helpscout.Conversation{}
}

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
		hostInfo := fmt.Sprintf("<br>Slug: %s<br>Region Identifier: %s<br>State: %v<br>Plan Name: %s<br>Subscription State: %s",
			host.Slug,
			host.RegionIdentifier.String,
			host.State,
			plan.Name,
			subscription.State)

		hostsInfo = append(hostsInfo, hostInfo)
	}
	workspaceInfo := strings.Join(hostsInfo, "<br>")

	billingProfile, err := env.User.BillingProfile(env.DB)
	if err != nil {
		return err
	}

	email := env.User.Email
	name := env.User.Name
	username := env.User.Username

	subject = fmt.Sprintf("[nitrous-pro] %s", subject)
	body = fmt.Sprintf("<strong>Username</strong>: %s\n<strong>Name</strong>: %s\n<strong>Email</strong>: %s\n%s\n", username, name, email, body)
	metadata := strings.Join([]string{
		fmt.Sprintf("<strong>User Agent</strong>: %s", userAgent),
		fmt.Sprintf("<strong>Platform</strong>: %s", platform),
		fmt.Sprintf("<strong>Current URL</strong>: %s", currentURL),
		fmt.Sprintf("<strong>User Info:</strong><br>ID: %v<br>Username: %s<br>Email: %s<br>Confirmed At: %v<br>Phone Authed: %t<br>Billing Profile State: %s",
			env.User.ID,
			username,
			email,
			env.User.ConfirmedAt.Time,
			env.User.PhoneAuthed,
			billingProfile.State),
		fmt.Sprintf("<strong>Workspace(s) Info</strong>: %s<br>", workspaceInfo),
	}, "<br>")

	customer := helpscout.Person{
		Email:     email,
		Username:  username,   // Optional
		FirstName: name,       // Optional
		Type:      "customer", // Optional
	}
	mailbox := helpscout.MailboxRef{
		ID:   envvars.HelpScoutMailboxID,
		Name: envvars.HelpScoutMailboxName, // Optional
	}
	threads := []*helpscout.Thread{
		&helpscout.Thread{
			Type: "customer",
			CreatedBy: helpscout.Person{
				Type:      "customer",
				Email:     email,
				Username:  username, // Optional
				FirstName: name,     // Optional
			},
			Body: body,
		},
		&helpscout.Thread{
			Type: "note",
			CreatedBy: helpscout.Person{
				Type: "user",
				ID:   int64(envvars.HelpScoutUserID),
			},
			Body: metadata,
		},
	}

	err = HelpScout.CreateConversation(customer, subject, mailbox, threads)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
	return nil
}
