package feedback

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
)

var (
	HelpScout helpscout.HelpScout

	nitrousMailbox helpscout.MailboxRef
	nitrousPerson  helpscout.Person
)

func init() {
	HelpScout = &helpscout.Conversation{}

	nitrousMailbox = helpscout.MailboxRef{ID: int(envvars.HelpScoutMailboxID), Name: envvars.HelpScoutMailboxName}
	nitrousPerson = helpscout.Person{Type: "user", ID: envvars.HelpScoutUserID}
}

func Send(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	subject := r.FormValue("subject")
	body := r.FormValue("body")

	if subject == "" && body == "" {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{"error": "invalid_request"}
		return nil
	}

	// Format email data and escape HTML in message body.
	subject = fmt.Sprintf("[nitrous-pro] %s", subject)
	body = html.EscapeString(body)
	metadata, err := prepareMetadata(env, r)
	if err != nil {
		logrus.Errorf("helpscout: failed to prepare metadata for nitrous note in new conversation: customer: %v, %s, %s, subject: %s, mailbox %s, customer message: %s, : err: %v", env.User.ID, env.User.Email, env.User.Username, subject, nitrousMailbox, body, err)
	}

	// Prepare HelpScout API arguments.
	customer := helpscout.NewCustomer(env.User.Email, env.User.Username, env.User.Name)
	customerMessage := helpscout.NewThread("customer", customer, body)
	nitrousNote := helpscout.NewThread("note", nitrousPerson, metadata)
	threads := []*helpscout.Thread{customerMessage, nitrousNote}

	go func() {
		err := HelpScout.CreateConversation(customer, subject, nitrousMailbox, threads)
		if err != nil {
			logrus.Errorf("helpscout: failed to create conversation: customer: %s, subject: %s, mailbox: %s, threads: %s. err: %v", customer, subject, nitrousMailbox, threads, err)
		}
	}()

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
	return nil
}

func prepareMetadata(env *middleware.Env, r *http.Request) (string, error) {
	workspaceInfo, err := getWorkspaceInfo(env.DB, env.User.ID)
	if err != nil {
		return "", err
	}
	billingProfile, err := env.User.BillingProfile(env.DB)
	if err != nil {
		return "", err
	}

	metadata := strings.Join([]string{
		fmt.Sprintf("<strong>Billing Profile State</strong>: %s", billingProfile.State),
		fmt.Sprintf("<br><strong>User Agent</strong>: %s", r.FormValue("userAgent")),
		fmt.Sprintf("<br><strong>Platform</strong>: %s", r.FormValue("platform")),
		fmt.Sprintf("<br><strong>Current URL</strong>: %s", r.FormValue("currentURL")),
		fmt.Sprintf("<br><strong>User Info</strong>: "),
		fmt.Sprintf("ID: %v, ", env.User.ID),
		fmt.Sprintf("Username: %s, ", env.User.Username),
		fmt.Sprintf("Email: %s, ", env.User.Email),
		fmt.Sprintf("Confirmed At: %s, ", env.User.ConfirmedAt.Time),
		fmt.Sprintf("Phone Authed: %t ", env.User.PhoneAuthed),
		fmt.Sprintf("<br><strong>Workspace(s) Info</strong>: %s", workspaceInfo),
	}, "")
	return metadata, nil
}

func getWorkspaceInfo(db db.QueryReceiver, userID int64) (string, error) {
	hosts, err := host.FindAllByUser(db, userID)
	if err != nil {
		return "", err
	}

	var hostsInfo []string
	for i, host := range hosts {
		plan, err := plan.FindByID(db, host.PlanID)
		if err != nil {
			return "", err
		}
		subscription, err := subscription.Find(db, host.SubscriptionID.Int64)
		if err != nil {
			return "", err
		}

		hostInfo := strings.Join([]string{
			fmt.Sprintf("%v.", i+1),
			fmt.Sprintf("Slug: %s,", host.Slug),
			fmt.Sprintf("Region Identifier: %s,", host.RegionIdentifier.String),
			fmt.Sprintf("State: %v,", host.State),
			fmt.Sprintf("Plan Name: %s,", plan.Name),
			fmt.Sprintf("Subscription State: %s", subscription.State),
		}, " ")
		hostsInfo = append(hostsInfo, hostInfo)
	}
	workspaceInfo := strings.Join(hostsInfo, "<br>")

	return workspaceInfo, nil
}
