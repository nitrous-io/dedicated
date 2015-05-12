package announcement

import (
	"bytes"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	templateLib "text/template"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	userLib "github.com/nitrous-io/dedicated/nitrous/models/user"
)

var (
	ErrSlugsNotFound = errors.New("Some of the host slugs cannot be found")
)

/*
 * HostOwner represents SQL-joined records of hosts and users table
 * For information about a host and its owner. It is used as
 * an embedded struct for host announcement.
 */
type HostOwner struct {
	HostID        int64       `db:"host_id"`
	HostCreatedAt pq.NullTime `db:"host_created_at"`
	UserID        int64       `db:"user_id"`
	Username      string      `db:"username"`
	Fullname      string      `db:"fullname"`
	Email         string      `db:"email"`
	Slug          string      `db:"slug"`
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
	ID              int64       `db:"id"`
	SubjectTemplate string      `db:"subject_template"`
	MessageTemplate string      `db:"message_template"`
	ShowToAll       bool        `db:"show_to_all"`
	ExpiresAt       pq.NullTime `db:"expires_at"`
	CreatedAt       pq.NullTime `db:"created_at"`
	UpdatedAt       pq.NullTime `db:"updated_at"`

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
	return ann.RenderSubject(ann.HostOwner)
}

func (ann *HostAnnouncement) Message() (string, error) {
	return ann.RenderMessage(ann.HostOwner)
}

func FindOwnerByHostID(db ndb.QueryReceiver, hostID int64) (*HostOwner, error) {
	owner := &HostOwner{}

	err := db.Get(owner, `SELECT
		hosts.id AS host_id, hosts.created_at AS host_created_at,
		users.id AS user_id, hosts.slug, username, users.name as fullname, email
		FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
		WHERE hosts.id=$1 AND users.deleted_at IS NULL;
	`, hostID)

	if err != nil {
		return nil, err
	}

	return owner, nil
}

func FindOwnersByHostSlugs(db *ndb.Database, slugs []string) ([]*HostOwner, error) {
	slugsCount := len(slugs)
	placeholders := make([]string, slugsCount)
	values := make([]interface{}, slugsCount)

	// Construct placeholder string for SELECT IN
	for i, slug := range slugs {
		values[i] = slug
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf(`SELECT
		hosts.id AS host_id, hosts.created_at AS host_created_at,
		users.id AS user_id, hosts.slug, username, users.name as fullname, email
		FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
		WHERE users.deleted_at IS NULL AND hosts.slug IN (%s);`, strings.Join(placeholders, ", "))

	owners := []*HostOwner{}
	if err := db.Select(&owners, query, values...); err != nil {
		return nil, err
	}

	return owners, nil
}

/*
 * Find all owners with active host, so that we can send global announcement to.
 */
func FindAllOwners(db *ndb.Database) ([]*HostOwner, error) {
	owners := []*HostOwner{}

	err := db.Select(&owners, `SELECT
		hosts.id AS host_id, users.id AS user_id, hosts.slug, username, users.name as fullname, email
		FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
		WHERE hosts.terminated_at IS NULL AND preprovisioned=false AND users.deleted_at IS NULL;
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
func FindByHostID(db *ndb.Database, hostID int64) ([]*HostAnnouncement, error) {
	// First find the owner, because it will be the same for all announcements
	owner, err := FindOwnerByHostID(db, hostID)
	if err != nil {
		return nil, err
	}

	announcements := []*Announcement{}

	// Query the database for announcements without joining
	// the hosts and users table
	err = db.Select(&announcements, `SELECT
		announcements.* FROM announcements
		LEFT JOIN announcement_host ON (announcements.id = announcement_host.announcement_id)
		WHERE expires_at > now()
			AND (show_to_all=true OR host_id=$1)
		ORDER BY created_at DESC;`, hostID)

	if err != nil {
		return nil, err
	}

	var hostAnnouncements []*HostAnnouncement

	// Manually join the same owner to each announcement to form HostAnnouncement
	for _, ann := range announcements {
		// exclude hosts created after announcement
		if owner.HostCreatedAt.Valid && ann.CreatedAt.Valid && owner.HostCreatedAt.Time.After(ann.CreatedAt.Time) {
			continue
		}
		hostAnnouncements = append(hostAnnouncements, &HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		})
	}

	return hostAnnouncements, nil
}

/*
 * Find all host-specific announcements
 */
func FindAllForHosts(db *ndb.Database) ([]*HostAnnouncement, error) {
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
			AND hosts.terminated_at IS NULL
			AND preprovisioned=false
			AND users.deleted_at IS NULL
		ORDER BY announcements.created_at DESC;`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

/*
 * Find all global announcements
 */
func FindAllForGlobal(db *ndb.Database) ([]*Announcement, error) {
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
 */
func FindAll(db *ndb.Database) ([]*HostAnnouncement, error) {
	// perform two separate queries to find
	// global and host announcements
	globals, err := FindAllForGlobal(db)
	if err != nil {
		return nil, err
	}

	hosts, err := FindAllForHosts(db)
	if err != nil {
		return nil, err
	}

	res := make([]*HostAnnouncement, len(globals))

	for i, ann := range globals {
		res[i] = &HostAnnouncement{ann, nil}
	}

	res = append(res, hosts...)

	return res, nil
}

/*
 * Insert an announcement without its associated host
 */
func Insert(db ndb.QueryReceiver, announcement *Announcement) error {
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

/*
 * Add an host to an existing announcement
 */
func AddHosts(db ndb.QueryReceiver, announcementID int64, hostIDs []int64) error {
	hostCount := len(hostIDs)

	placeholders := make([]string, hostCount)
	values := make([]interface{}, 2*hostCount)

	// Construct query string for bulk insert hosts
	for i, hostID := range hostIDs {
		offset := i * 2

		values[offset] = announcementID
		values[offset+1] = hostID

		placeholders[i] = fmt.Sprintf("($%d, $%d)", offset+1, offset+2)
	}

	query := fmt.Sprintf("INSERT INTO announcement_host (announcement_id, host_id) VALUES %s;",
		strings.Join(placeholders, ", "))

	_, err := db.Exec(query, values...)

	return err
}

/*
 * Insert announcement associated with given hosts.
 */
func InsertForHosts(db *ndb.Database, ann *Announcement, hostIDs []int64) error {
	return db.WithTransaction(func(tx *sqlx.Tx) error {
		if err := Insert(tx, ann); err != nil {
			return err
		}

		return AddHosts(tx, ann.ID, hostIDs)
	})
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

	uName := owner.Fullname
	if uName == "" {
		uName = owner.Username
	}

	msgBody := fmt.Sprintf("Hello %s,\n\n%s\n\nThanks,\nThe Nitrous Team", uName, message)

	from := emailAddress("Nitrous Pro", "mailer@nitrous.io")
	to := emailAddress(owner.Fullname, owner.Email)

	if err := sendEmail(from, to, subject, msgBody); err != nil {
		return err
	}

	return nil
}

func SendToOwners(ann *Announcement, owners []*HostOwner, errorHandler func(error)) {
	go func() {
		for _, owner := range owners {
			if err := SendToOwner(ann, owner); err != nil {
				errorHandler(err)
			}
		}
	}()
}

func SendAndSaveGlobalAnnouncement(db *ndb.Database, ann *Announcement, errHandler func(error)) error {
	owners, err := FindAllOwners(db)
	if err != nil {
		return err
	}

	if err := Insert(db, ann); err != nil {
		return err
	}

	SendToOwners(ann, owners, errHandler)

	return nil
}

func SendAndSaveForHostSlugs(db *ndb.Database, ann *Announcement, slugs []string, errHandler func(error)) ([]*HostAnnouncement, error) {
	slugsCount := len(slugs)

	owners, err := FindOwnersByHostSlugs(db, slugs)
	if err != nil {
		return nil, err
	}

	if len(owners) != slugsCount {
		return nil, ErrSlugsNotFound
	}

	hostAnns := make([]*HostAnnouncement, slugsCount)
	hostIDs := make([]int64, slugsCount)

	for i, owner := range owners {
		hostAnns[i] = &HostAnnouncement{
			Announcement: ann,
			HostOwner:    owner,
		}

		hostIDs[i] = owner.HostID
	}

	if err := InsertForHosts(db, ann, hostIDs); err != nil {
		return nil, err
	}

	SendToOwners(ann, owners, errHandler)

	return hostAnns, nil
}
