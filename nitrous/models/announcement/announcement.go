package announcement

import (
	"bytes"
	"fmt"
	"net/mail"
	templateLib "text/template"
	"time"

	"github.com/jmoiron/sqlx"
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

func (ann *Announcement) IsGlobal() bool {
	return ann.ShowToAll
}

func (ann *Announcement) RenderSubject(owner *HostOwner) (string, error) {
	return renderTemplate(&ann.compiledSubjectTemplate, "AnnouncementSubject", ann.SubjectTemplate, owner)
}

func (ann *Announcement) RenderMessage(owner *HostOwner) (string, error) {
	return renderTemplate(&ann.compiledMessageTemplate, "AnnouncementMessage", ann.MessageTemplate, owner)
}

func (ann *HostAnnouncement) Subject() (string, error) {
	return ann.Announcement.RenderSubject(ann.HostOwner)
}

func (ann *HostAnnouncement) Message() (string, error) {
	return ann.Announcement.RenderMessage(ann.HostOwner)
}

func (ann *HostAnnouncement) SendEmail() error {
	return SendToOwner(ann.Announcement, ann.HostOwner)
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

func FindOwnerByHostSlug(db *dbLib.Database, slug string) (*HostOwner, error) {
	owner := &HostOwner{}

	err := db.Get(owner, `SELECT
    hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.slug=$1;
  `, slug)

	if err != nil {
		return nil, err
	}

	return owner, nil
}

func FindOwnersByHostSlugs(db *dbLib.Database, slugs []string) ([]*HostOwner, error) {
	query := `SELECT
    hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.slug IN (`

	slugsCount := len(slugs)
	values := make([]interface{}, slugsCount)

	// Ugly hacks to perform SQL SELECT IN query
	for i, slug := range slugs {
		values[i] = slug
		query += fmt.Sprintf("$%d", i+1)
		if i+1 != slugsCount {
			query += ", "
		}
	}

	query += ");"

	owners := []*HostOwner{}

	err := db.Select(&owners, query, values...)

	if err != nil {
		return nil, err
	}

	return owners, nil
}

/*
 * Find all owners with active host, so that we can send global announcement to.
 */
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
    announcements.* FROM announcements 
    LEFT JOIN announcement_host ON (announcements.id = announcement_host.announcement_id)
    WHERE expires_at > now() 
      AND (show_to_all=true OR host_id=$1)
    ORDER BY created_at DESC;`, hostID)

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
func FindAllForHosts(db *dbLib.Database) ([]*HostAnnouncement, error) {
	announcements := []*HostAnnouncement{}

	err := db.Select(&announcements, `SELECT 
    announcements.*, hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email 
    FROM announcements 
      INNER JOIN announcement_host ON (announcements.id = announcement_host.announcement_id)
      INNER JOIN hosts ON (announcement_host.host_id = hosts.id)
      INNER JOIN users ON (hosts.user_id = users.id)
    WHERE 
      show_to_all=false
      AND expires_at > now() 
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
		FROM announcements 
    WHERE show_to_all=true AND expires_at > now();`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

/*
 * Find all global and host-specific announcements.
 * Global announcements would have HostOwner field as nil.
 *
 * Soares: We have to perform two separate queries to find
 * global and host announcements, because I have no idea
 * how to map left joined rows with null-host_id to
 * this HostAnnouncement struct.. :(
 */
func FindAll(db *dbLib.Database) ([]*HostAnnouncement, error) {
	globals, err := FindAllForGlobal(db)
	if err != nil {
		return nil, err
	}

	hosts, err := FindAllForHosts(db)
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

type prepareNamedDB interface {
	PrepareNamed(query string) (*sqlx.NamedStmt, error)
}

/*
 * Insert an announcement without its associated host
 */
func Insert(db prepareNamedDB, announcement *Announcement) error {
	query := `INSERT INTO announcements
    (subject_template, message_template, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject_template, :message_template, :show_to_all, :expires_at, now(), now())
    RETURNING *;`

	nstmt, err := db.PrepareNamed(query)
	if err != nil {
		return err
	}

	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

func AddHost(db *dbLib.Database, announcementID, hostID int64) error {
	_, err := db.Exec(`INSERT INTO announcement_host
    (announcement_id, host_id) 
    VALUES ($1, $2)`,
		announcementID, hostID)

	return err
}

/*
 * Add an host to an existing announcement
 */
func AddHosts(db *sqlx.Tx, announcementID int64, hostIDs []int64) error {
	hostCount := len(hostIDs)

	query := "INSERT INTO announcement_host (announcement_id, host_id) VALUES "
	values := make([]interface{}, 2*hostCount)

	// Ugly hack for inserting multiple rows at the same time in Go..
	for i, hostID := range hostIDs {
		offset := i * 2

		values[offset] = announcementID
		values[offset+1] = hostID

		query += fmt.Sprintf("($%d, $%d)", offset+1, offset+2)

		if i+1 != hostCount {
			query += ", "
		}
	}

	query += ";"

	_, err := db.Exec(query, values...)

	return err
}

/*
 * Insert announcement associated with given hosts.
 */
func InsertForHosts(db *dbLib.Database, ann *Announcement, hostIDs []int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = Insert(tx, ann)
	if err != nil {
		return err
	}

	err = AddHosts(tx, ann.ID, hostIDs)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func InsertForHost(db *dbLib.Database, ann *Announcement, hostID int64) error {
	err := Insert(db, ann)
	if err != nil {
		return err
	}

	err = AddHost(db, ann.ID, hostID)
	if err != nil {
		return err
	}

	return nil
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

func emailAddress(name string, address string) string {
	return (&mail.Address{Name: name, Address: address}).String()
}

func sendEmail(from, to, subject, body string) error {
	return userLib.Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, body, "")
}

func SendToOwner(ann *Announcement, owner *HostOwner) error {
	subject, err := ann.RenderSubject(owner)
	if err != nil {
		return err
	}

	message, err := ann.RenderMessage(owner)
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

func SendAndSaveGlobalAnnouncement(db *dbLib.Database, ann *Announcement, errHandler func(error)) error {
	owners, err := FindAllOwners(db)
	if err != nil {
		return err
	}

	err = Insert(db, ann)
	if err != nil {
		return err
	}

	SendToOwners(ann, owners, errHandler)

	return nil
}

func SendAndSaveForHostSlugs(db *dbLib.Database, ann *Announcement, slugs []string, errHandler func(error)) ([]*HostAnnouncement, error) {
	owners, err := FindOwnersByHostSlugs(db, slugs)
	if err != nil {
		return nil, err
	}

	hostAnns := make([]*HostAnnouncement, len(owners))
	hostIDs := make([]int64, len(owners))

	for i, owner := range owners {
		hostAnns[i] = &HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		}

		hostIDs[i] = owner.HostID
	}

	err = InsertForHosts(db, ann, hostIDs)
	if err != nil {
		return nil, err
	}

	SendToOwners(ann, owners, errHandler)

	return hostAnns, nil
}
