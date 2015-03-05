package announcement

import (
	"bytes"
	"net/mail"
	templateLib "text/template"
	"time"

	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
	userLib "github.com/nitrous-io/dedicated/nitrous/models/user"
)

type HostOwner struct {
	HostID   int64  `db:"host_id"`
	UserID   int64  `db:"user_id"`
	Username string `db"username"`
	Fullname string `db:"fullname"`
	Email    string `db:"email"`
	Slug     string `db:"slug"`
}

type Announcement struct {
	ID              int64      `db:"id"`
	SubjectTemplate string     `db:"subject_template"`
	MessageTemplate string     `db:"message_template"`
	ShowToAll       bool       `db:"show_to_all"`
	ExpiresAt       *time.Time `db:"expires_at"`
	CreatedAt       *time.Time `db:"created_at"`
	UpdatedAt       *time.Time `db:"updated_at"`

	compiledSubjectTemplate *templateLib.Template
	compiledMessageTemplate *templateLib.Template
}

type HostAnnouncement struct {
	*Announcement
	*HostOwner
}

func CreateForGlobal(subjectTemplate, messageTemplate string, expiresAt *time.Time) *Announcement {
	return &Announcement{
		SubjectTemplate: subjectTemplate,
		MessageTemplate: messageTemplate,
		ShowToAll:       true,
		ExpiresAt:       expiresAt,
	}
}

func CreateForHostOwner(owner *HostOwner, subjectTemplate, messageTemplate string, expiresAt *time.Time) *HostAnnouncement {
	return &HostAnnouncement{
		Announcement: &Announcement{
			SubjectTemplate: subjectTemplate,
			MessageTemplate: messageTemplate,
			ShowToAll:       false,
			ExpiresAt:       expiresAt,
		},
		HostOwner: owner,
	}
}

func CreateForHost(db *dbLib.Database, hostID int64, subjectTemplate, messageTemplate string, expiresAt *time.Time) (*HostAnnouncement, error) {
	owner, err := FindOwnerByHostID(db, hostID)
	if err != nil {
		return nil, err
	}

	return CreateForHostOwner(owner, subjectTemplate, messageTemplate, expiresAt), nil
}

func FindOwnerByHostID(db *dbLib.Database, hostID int64) (*HostOwner, error) {
	owner := &HostOwner{}

	err := db.Get(owner, `SELECT
    hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.id=$1;
  `, hostID)

	if err != nil {
		return nil, err
	}

	return owner, nil
}

func FindAllOwners(db *dbLib.Database) ([]*HostOwner, error) {
	owners := []*HostOwner{}

	err := db.Select(&owners, `SELECT
    hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.terminated_at IS NULL;
  `)

	if err != nil {
		return nil, err
	}

	return owners, nil
}

func FindAllForHost(db *dbLib.Database, hostID int64) ([]*HostAnnouncement, error) {
	owner, err := FindOwnerByHostID(db, hostID)
	if err != nil {
		return nil, err
	}

	announcements := []*Announcement{}

	err = db.Select(&announcements, `SELECT 
    id, subject_template, message_template, show_to_all, expires_at, updated_at, created_at
    FROM announcements 
    WHERE host_id IS NULL OR host_id=$1;`, hostID)

	if err != nil {
		return nil, err
	}

	hostAnnouncements := make([]*HostAnnouncement, len(announcements))

	for i, ann := range announcements {
		hostAnnouncements[i] = &HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		}
	}

	return hostAnnouncements, nil
}

func FindForHost(db *dbLib.Database, hostID int64) ([]*HostAnnouncement, error) {
	announcements := []*HostAnnouncement{}

	err := db.Select(&announcements, `SELECT 
    announcements.*, hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM announcements 
      INNER JOIN hosts ON (announcements.host_id = hosts.id)
      INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.terminated_at IS NULL AND hosts.id=$1;`, hostID)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

func FindForGlobal(db *dbLib.Database) ([]*Announcement, error) {
	announcements := []*Announcement{}

	err := db.Select(&announcements, `SELECT 
		id, subject, message, show_to_all, expires_at, created_at, updated_at
		FROM announcements WHERE show_to_all=true;`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

func InsertForGlobal(db *dbLib.Database, announcement *Announcement) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO announcement
    (subject, message, host_id, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject, :message, NULL, true, :expires_at, now(), now())
    RETURNING *;`)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

func InsertForHost(db *dbLib.Database, announcement *HostAnnouncement) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO announcement
    (subject, message, host_id, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject, :message, :host_id, false, :expires_at, now(), now())
    RETURNING *;`)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

func renderTemplate(templateCache **templateLib.Template, templateName, templateText string, ctx interface{}) (string, error) {
	var err error
	template := *templateCache

	if template == nil {
		template, err = templateLib.New(templateName).Parse(templateText)
		if err != nil {
			return "", err
		}

		*templateCache = template
	}

	buf := bytes.NewBuffer(nil)
	template.Execute(buf, ctx)

	return buf.String(), nil
}

func (ann *Announcement) IsGlobal() bool {
	return ann.ShowToAll
}

func (ann *Announcement) Subject(owner *HostOwner) (string, error) {
	return renderTemplate(&ann.compiledSubjectTemplate, "AnnouncementSubject", ann.SubjectTemplate, owner)
}

func (ann *Announcement) Message(owner *HostOwner) (string, error) {
	return renderTemplate(&ann.compiledMessageTemplate, "AnnouncementMessage", ann.MessageTemplate, owner)
}

func (ann *HostAnnouncement) Subject() (string, error) {
	return ann.Announcement.Subject(ann.HostOwner)
}

func (ann *HostAnnouncement) Message() (string, error) {
	return ann.Announcement.Message(ann.HostOwner)
}

func (ann *HostAnnouncement) SendEmail() error {
	return SendToOwner(ann.Announcement, ann.HostOwner)
}

func emailAddress(name string, address string) string {
	return (&mail.Address{Name: name, Address: address}).String()
}

func sendEmail(from, to, subject, body string) error {
	return userLib.Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, body, "")
}

func SendToOwner(ann *Announcement, owner *HostOwner) error {
	subject, err := ann.Subject(owner)
	if err != nil {
		return err
	}

	message, err := ann.Message(owner)
	if err != nil {
		return err
	}

	from := emailAddress("Nitrous Mailer", "mailer@nitrous.io")
	to := emailAddress(owner.Fullname, owner.Email)

	err = sendEmail(from, to, subject, message)
	if err != nil {
		return err
	}

	return nil
}

func SendToOwners(ann *Announcement, owners []*HostOwner, errorHandler func(error)) {
	go func() {
		for _, owner := range owners {
			err := SendToOwner(ann, owner)
			if err != nil {
				errorHandler(err)
			}
		}
	}()
}

func SendAndSaveGlobalAnnouncement(db *dbLib.Database, ann *Announcement) error {
	owners, err := FindAllOwners(db)
	if err != nil {
		return err
	}

	err = InsertForGlobal(db, ann)
	if err != nil {
		return err
	}

	// ignore error for now
	SendToOwners(ann, owners, func(error) {})

	return nil
}
