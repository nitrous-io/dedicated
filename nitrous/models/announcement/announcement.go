package announcement

import (
	"bytes"
	"net/mail"
	templateLib "text/template"
	"time"

	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
	userLib "github.com/nitrous-io/dedicated/nitrous/models/user"
)

/*
 * HostOwner represents SQL-joined records of hosts and users table
 * For information about a host and its owner. It is used as
 * an embedded struct for host announcement.
 */
type HostOwner struct {
	HostID   int64  `db:"host_id"`
	UserID   int64  `db:"user_id"`
	Username string `db"username"`
	Fullname string `db:"fullname"`
	Email    string `db:"email"`
	Slug     string `db:"slug"`
}

/*
 * Announcement contains the base fields inside the
 * announcement table. It has no link to specific host.
 * The ShowToAll field is used to indicate whether
 * the announcement is global or host-specific.
 *
 * Announcement can be used alone if it is a global
 * announcement. Otherwise it is embedded to
 * HostAnnouncement to contain full record of
 * an announcement together with its host and owner.
 */
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

/*
 * Use to represent an announcement with host and
 * owner information. Note that HostAnnouncement
 * can also be used to represent global announcement,
 * in which case *HostOwner will be nil and ShowToAll
 * should be true.
 */
type HostAnnouncement struct {
	*Announcement
	*HostOwner
}

// Create a global Announcement object
func CreateForGlobal(subjectTemplate, messageTemplate string, expiresAt *time.Time) *Announcement {
	return &Announcement{
		SubjectTemplate: subjectTemplate,
		MessageTemplate: messageTemplate,
		ShowToAll:       true,
		ExpiresAt:       expiresAt,
	}
}

// Create a HostAnnouncement object from given host owner
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

// Create a HostAnnouncement object from given host id
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

/*
 * Find both global and host announcements for a given host id.
 * The global announcements retrieved are assigned the same
 * host owner as the hosts.
 */
func FindAllByHostID(db *dbLib.Database, hostID int64) ([]*HostAnnouncement, error) {
	// First find the owner, because it will be the same for all announcements
	owner, err := FindOwnerByHostID(db, hostID)
	if err != nil {
		return nil, err
	}

	announcements := []*Announcement{}

	// Query the database for announcements without joining
	err = db.Select(&announcements, `SELECT 
    announcements.id, subject_template, message_template, show_to_all, expires_at, updated_at, created_at
    FROM announcements 
    LEFT JOIN announcement_host ON (announcements.id = announcement_host.announcement_id)
    WHERE expires_at > now() 
      AND (show_to_all=true OR host_id=$1);`, hostID)

	if err != nil {
		return nil, err
	}

	hostAnnouncements := make([]*HostAnnouncement, len(announcements))

	// Manually join the same owner to each announcement to form HostAnnouncement
	for i, ann := range announcements {
		hostAnnouncements[i] = &HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		}
	}

	return hostAnnouncements, nil
}

/*
 * Find all host-specific announcements
 */
func FindAllForHost(db *dbLib.Database) ([]*HostAnnouncement, error) {
	announcements := []*HostAnnouncement{}

	err := db.Select(&announcements, `SELECT 
    announcements.*, hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM announcements 
      INNER JOIN announcement_host ON (announcements.id = announcement_host.announcement_id)
      INNER JOIN hosts ON (announcement_host.host_id = hosts.id)
      INNER JOIN users ON (hosts.user_id = users.id)
    WHERE 
      show_to_all=false
      AND hosts.terminated_at IS NULL;`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

/*
 * Find all global announcements
 */
func FindAllForGlobal(db *dbLib.Database) ([]*Announcement, error) {
	announcements := []*Announcement{}

	err := db.Select(&announcements, `SELECT *
		FROM announcements WHERE show_to_all=true;`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

/*
 * Find all global and host-specific announcements.
 * Global announcements would have HostOwner field as nil
 */
func FindAll(db *dbLib.Database) ([]*HostAnnouncement, error) {
	globals, err := FindAllForGlobal(db)
	if err != nil {
		return nil, err
	}

	hosts, err := FindAllForHost(db)
	if err != nil {
		return nil, err
	}

	totalLen := +len(hosts)
	res := make([]*HostAnnouncement, totalLen)

	for i, ann := range globals {
		res[i] = &HostAnnouncement{ann, nil}
	}

	return append(res, hosts...), nil
}

/*
 * Insert an announcement without its associated host, usually for global
 */
func Insert(db *dbLib.Database, announcement *Announcement) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO announcements
    (subject, message, host_id, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject, :message, NULL, :show_to_all, :expires_at, now(), now())
    RETURNING *;`)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

/*
 * Add an host to an announcement
 */
func AddHost(db *dbLib.Database, announcementID, hostID int64) error {
	_, err := db.Exec(`INSERT INTO announcement_host
    (announcement_id, host_id) 
    VALUES ($1, $2)`,
		announcementID, hostID)

	return err
}

/*
 * Insert a host-specific announcement
 */
func InsertForHost(db *dbLib.Database, ann *HostAnnouncement) error {
	err := Insert(db, ann.Announcement)
	if err != nil {
		return err
	}

	return AddHost(db, ann.ID, ann.HostID)
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

	err = Insert(db, ann)
	if err != nil {
		return err
	}

	// ignore error for now
	SendToOwners(ann, owners, func(error) {})

	return nil
}
