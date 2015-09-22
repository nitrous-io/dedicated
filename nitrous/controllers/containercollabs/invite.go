package containercollabs

import (
	"bytes"
	"database/sql"
	"fmt"
	"html"
	"net/url"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

func inviteUser(d *db.Database, c *container.Container, email string, sharer *user.User) (err error) {
	sc := &containercollab.ContainerCollab{
		ContainerID: c.ID,
		InviteEmail: sql.NullString{email, true},
	}
	t, err := securerandom.Base64(15)
	if err != nil {
		return
	}
	token := fmt.Sprintf("%s", t[:20])
	sc.InviteCode = sql.NullString{token, true}

	return d.WithTransaction(func(tx *sqlx.Tx) error {
		nstmt, err := tx.PrepareNamed(`INSERT INTO container_collabs (container_id, invite_email, invite_code, email_sent_at)
		  VALUES (:container_id, :invite_email, :invite_code, now()) RETURNING *;`)
		if err != nil {
			return err
		}
		err = nstmt.QueryRowx(sc).StructScan(sc)
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
			Subject       string
			Email         string
			SharerName    string
			ContainerSlug string
			SignupUrl     string
		}{
			"",
			email,
			html.EscapeString(sharer.DisplayName()),
			c.Slug,
			urlString,
		}

		tmplData.Subject = fmt.Sprintf("%s has shared container %s with you!", tmplData.SharerName, tmplData.ContainerSlug)

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
