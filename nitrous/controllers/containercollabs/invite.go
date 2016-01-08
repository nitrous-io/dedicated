package containercollabs

import (
	"bytes"
	"fmt"
	"html"
	"net/url"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func inviteUser(d *db.Database, c *container.Container, email string, sharer *user.User) (err error) {
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		sc, err := containercollab.CreatePendingMember(tx, c.ID, email)
		if err != nil {
			return err
		}

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
		query.Set("invite", sc.InviteCode.String)
		urlString := fmt.Sprintf("%s?%s", baseUrl, query.Encode())

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
