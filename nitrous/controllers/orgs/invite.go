package orgs

import (
	"bytes"
	"fmt"
	"html"
	"net/url"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func inviteUser(d *db.Database, o *org.Org, email string, owner *user.User) error {
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		// Don't add member if not notified by email
		token, err := orgmembership.AddPendingMember(tx, o.ID, email, "member")
		if err != nil {
			return err
		}

		return sendEmail(o, email, owner, token)
	})
}

func sendEmail(o *org.Org, email string, owner *user.User, token string) error {
	host := envvars.WebHostname
	if host == "" {
		host = "www.nitrous.io"
	}
	baseUrl := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app/",
		Fragment: "/signup",
	}
	query := url.Values{}
	query.Set("invite", token)
	query.Add("org", "true")
	query.Encode()

	urlString := fmt.Sprintf("%s?%s", baseUrl.String(), query)

	tmplData := struct {
		Subject   string
		Email     string
		OwnerName string
		OrgName   string
		SignupUrl string
	}{
		"",
		email,
		html.EscapeString(owner.DisplayName()),
		o.Name,
		urlString,
	}

	tmplData.Subject = fmt.Sprintf("%s has invited you to organization %s!", tmplData.OwnerName, tmplData.OrgName)

	htmlBuf := bytes.NewBuffer(nil)
	if err := invitedHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := invitedTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	from := envvars.SupportEmail
	return Mailer.SendMail(from, []string{email}, []string{from}, nil, from, tmplData.Subject, txtBuf.String(), htmlBuf.String())

}
