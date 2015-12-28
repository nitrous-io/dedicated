package orgs

import (
	"bytes"
	"fmt"
	"html"
	"net/url"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func inviteUser(d *db.Database, o *org.Org, invitecode, email string, owner *user.User) error {
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
	query.Set("invite", invitecode)
	urlString := fmt.Sprintf("%s?%s", baseUrl, query.Encode())

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
