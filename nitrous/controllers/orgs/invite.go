package orgs

import (
	"bytes"
	"database/sql"
	"fmt"
	"html"
	"net/url"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

func inviteUser(d *db.Database, o *org.Org, email string, owner *user.User) (err error) {
	om := &orgmembership.OrgMembership{
		OrgID:       o.ID,
		InviteEmail: sql.NullString{email, true},
	}
	t, err := securerandom.Alphanumeric(20)
	if err != nil {
		return
	}
	token := fmt.Sprintf("%s", t[:20])
	om.InviteCode = sql.NullString{token, true}

	return d.WithTransaction(func(tx *sqlx.Tx) error {
		nstmt, err := tx.PrepareNamed(`INSERT INTO org_memberships (org_id, invite_email, invite_code, email_sent_at)
		VALUES (:org_id, :invite_email, :invite_code, now()) RETURNING *;`)
		if err != nil {
			return err
		}
		err = nstmt.QueryRowx(om).StructScan(om)
		if err != nil {
			return err
		}

		host := envvars.WebHostname
		if host == "" {
			host = "www.nitrous.io"
		}
		url := &url.URL{
			Scheme:   "https",
			Host:     host,
			Path:     "/app/",
			Fragment: "/signup",
		}
		urlString := fmt.Sprintf("%s?email=%s&invite_code=%s", url.String(), email, token)

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
		if err = invitedHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
			return err
		}
		txtBuf := bytes.NewBuffer(nil)
		if err = invitedTxtTemplate.Execute(txtBuf, tmplData); err != nil {
			return err
		}

		from := envvars.SupportEmail
		return Mailer.SendMail(from, []string{email}, []string{from}, nil, from, tmplData.Subject, txtBuf.String(), htmlBuf.String())
	})
}
