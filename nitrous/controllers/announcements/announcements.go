package announcements

import (
	"bytes"
	"net/http"
	"net/mail"
	templateLib "text/template"
	"time"

	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	userLib "github.com/nitrous-io/dedicated/nitrous/models/user"
)

func ShowGlobalAnnouncements(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindForGlobal(env.DB)
	if err != nil {
		return err
	}

	env.Logger.Infof("global announcements: %+v", announcements)

	env.Status = http.StatusOK
	return nil
}

func renderTemplate(templateName, templateText string, ctx interface{}) (string, error) {
	template, err := templateLib.New(templateName).Parse(templateText)
	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(nil)
	template.Execute(buf, ctx)

	return buf.String(), nil
}

func emailAddress(name string, address string) string {
	return (&mail.Address{Name: name, Address: address}).String()
}

func sendEmail(from, to, subject, body string) error {
	return userLib.Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, body, "")
}

func SendAndSaveHostAnnouncement(db *dbLib.Database, owner *announcement.HostOwner, ExpiresAt *time.Time, subject, bodyTemplate string) error {
	body, err := renderTemplate("hostAnnouncement", bodyTemplate, owner)
	if err != nil {
		return err
	}

	ann := &announcement.HostAnnouncement{
		announcement.Announcement{
			Subject:   subject,
			Message:   body,
			ShowToAll: false,
			ExpiresAt: ExpiresAt,
		},
		*owner,
	}

	err = announcement.InsertForHost(db, ann)
	if err != nil {
		return err
	}

	from := emailAddress("Nitrous Mailer", "mailer@nitrous.io")
	to := emailAddress(owner.Fullname, owner.Email)

	err = sendEmail(from, to, subject, body)
	if err != nil {
		return err
	}

	return nil
}

func SendAndSaveGlobalAnnouncement(db *dbLib.Database, ExpiresAt *time.Time, subject, message string) error {
	owners, err := announcement.FindAllHostOwners(db)
	if err != nil {
		return err
	}

	ann := &announcement.Announcement{
		Subject:   subject,
		Message:   message,
		ShowToAll: true,
	}

	err = announcement.InsertForGlobal(db, ann)
	if err != nil {
		return err
	}

	from := emailAddress("Nitrous Mailer", "mailer@nitrous.io")

	for _, owner := range owners {
		to := emailAddress(owner.Fullname, owner.Email)
		err = sendEmail(from, to, subject, message)
		if err != nil {
			// log error and proceed with other emails
		}
	}

	// ignore errors sending individual email after logging them
	return nil
}
