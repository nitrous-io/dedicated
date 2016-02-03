package containercollabs

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	Index  middleware.Middleware = containers.WithContainerAndHost(index, containers.AllowCollab)
	Add    middleware.Middleware = containers.WithContainerAndHost(add, containers.TouchHost)
	Remove middleware.Middleware = containers.WithContainerAndHost(remove, containers.AllowCollab, containers.TouchHost)

	errMissingUserParam     = httperror.New(http.StatusBadRequest, "invalid_request", "`user` must be provided")
	errUserNotFound         = httperror.New(http.StatusBadRequest, "user_not_found", "the specified user could not be found")
	errSameUser             = httperror.New(http.StatusBadRequest, "same_user", "you cannot add yourself as a collaborator")
	errAlreadyAdded         = httperror.New(http.StatusConflict, "already_added", "this user is already a collaborator")
	errUserNotConfirmed     = httperror.New(http.StatusBadRequest, "user_not_confirmed", "the specified user has not confirmed their account")
	errCollaboratorNotFound = httperror.New(http.StatusNotFound, "collaborator_not_found", "the specified collaborator was not found")
	errRemoveForbidden      = httperror.New(http.StatusForbidden, "invalid_request", "you do not have permission to remove this collaborator")
	errCollabLimitReached   = httperror.New(http.StatusForbidden, "collab_limit_reached", "you have reached the maximum number of collaborators under your current plan")
	errEmailAlreadySent     = errors.New("email already sent")
	Mailer                  mailer.Mailer
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

func index(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	sc, err := containercollab.FindAll(env.DB, cn.ID)
	if err != nil {
		return err
	}

	collabs := serializers.SerializeContainerCollabUsers(sc)

	// we include the owner so we don't have to perform additional requests for active files
	// on the IDE
	if env.User.ID != h.UserID {
		u, err := user.FindByID(env.DB, h.UserID)
		if err != nil {
			logrus.Errorf("Unable to fetch container owner %d, err: %v", h.UserID, err)
		} else {
			// fake the user for serialization
			cc := &containercollab.ContainerCollabUser{
				ContainerCollab: containercollab.ContainerCollab{
					ContainerID: cn.ID,
					UserID:      sql.NullInt64{u.ID, true},
					CreatedAt:   cn.CreatedAt,
					UpdatedAt:   cn.UpdatedAt,
				},
				Username: sql.NullString{u.Username, true},
				Email:    sql.NullString{u.Email, true},
			}
			if u.Name != "" {
				cc.Name = sql.NullString{u.Name, true}
			}
			ser := serializers.SerializeContainerCollabUser(cc)
			ser.Owner = true
			collabs = append(collabs, ser)
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"collaborators": collabs,
	}
	return nil
}

func add(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if err := containercollab.CanAddCollaborator(env.DB, cn.ID); err != nil {
		if err == containercollab.ErrCollabLimitReached {
			return errCollabLimitReached
		}
		return err
	}

	username := r.FormValue("user")
	if username == "" {
		return errMissingUserParam
	}

	noEmail := (r.FormValue("noemail") == "true")

	usr, err := user.FindByUsernameOrEmail(env.DB, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return handleNoUser(cn, env, username)
		}
		return err
	}

	if usr.ServiceAccount {
		return errUserNotFound
	}

	if usr.ID == env.User.ID {
		return errSameUser
	}

	if !usr.CanLogin() {
		return errUserNotConfirmed
	}

	collab, err := containercollab.Create(env.DB, cn.ID, strconv.FormatInt(usr.ID, 10))
	if err != nil {
		pqErr, ok := err.(*pq.Error)
		if ok && strings.Contains(pqErr.Message, "duplicate key") {
			return errAlreadyAdded
		}
		return err
	}

	if !noEmail {
		go func() {
			err := sendEmail(env.DB, cn, collab, env.User, usr)
			if err != nil {
				logrus.Errorf("Failed to send email for collaborator %d, user id: %d err: %+v", collab.ID, env.User.ID, err)
			}
		}()
	}

	collabData := map[string]interface{}{
		"collaborator": map[string]interface{}{
			"id":       usr.ID,
			"username": usr.Username,
		},
	}
	containers.PublishContainerEvent(env, controllers.EventCollaboratorAdded, cn, collabData)

	env.Tracker.SetProfileProps("inc", env.User.ID, map[string]interface{}{
		"collab_invites_count": 1,
	})

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"success": true,
	}
	return nil
}

func remove(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	ccid, err := strconv.ParseInt(mux.Vars(r)["ccid"], 10, 64)
	if err != nil {
		return err
	}

	var affectedUserID sql.NullInt64
	err = env.DB.Get(&affectedUserID, `SELECT user_id FROM container_collabs WHERE id = $1`, ccid)
	if err != nil {
		if err == sql.ErrNoRows {
			return errCollaboratorNotFound
		}
		return err
	}

	if !isOwner(env, cn) && !isSelf(env, affectedUserID.Int64) && !env.User.Admin {
		return errRemoveForbidden
	}
	if err = containercollab.Delete(env.DB, ccid, cn.ID); err != nil {
		return err
	}

	// Don't publish events to removed users that are pending invite since they don't have an account.
	collabData := make(map[string]interface{})
	if affectedUserID.Valid {
		collabData, err = publishRemoveEvent(env, cn, affectedUserID.Int64)
		if err != nil {
			return err
		}
	}
	containers.PublishContainerEvent(env, controllers.EventCollaboratorRemoved, cn, collabData)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"success": true,
	}
	return nil
}

func isOwner(env *middleware.Env, cn *container.Container) bool {
	return env.User.ID == cn.UserID
}

func isSelf(env *middleware.Env, uID int64) bool {
	return env.User.ID == uID
}

func publishRemoveEvent(env *middleware.Env, cn *container.Container, uID int64) (map[string]interface{}, error) {
	var username string
	err := env.DB.Get(&username, `SELECT username FROM users WHERE id = $1`, uID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errUserNotFound
		}
		return nil, err
	}

	collabData := map[string]interface{}{
		"collaborator": map[string]interface{}{
			"id":       uID,
			"username": username,
		},
	}

	// Fire to removed user
	topic := pubsub.Topic(fmt.Sprintf("user-%d", uID))
	data := map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
		"initiated_by": map[string]interface{}{
			"username": env.User.Username,
			"id":       env.User.ID,
		},
		"collaborator": collabData["collaborator"],
	}
	env.Events.Publish(controllers.NewEvent(controllers.EventCollaboratorRemoved, data), topic)

	return collabData, nil
}

func handleNoUser(cn *container.Container, env *middleware.Env, email string) error {
	if !strings.Contains(email, "@") || blacklist.Check(email) {
		return errUserNotFound
	}

	err := inviteUser(env.DB, cn, email, env.User)
	if err != nil {
		return err
	}

	collabData := map[string]interface{}{
		"collaborator": map[string]interface{}{
			"username": email,
		},
	}
	containers.PublishContainerEvent(env, controllers.EventCollaboratorAdded, cn, collabData)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"success": true,
	}
	return nil
}

func sendEmail(d *db.Database, c *container.Container, collab *containercollab.ContainerCollab, sharer *user.User, recv *user.User) (err error) {
	if collab.EmailSentAt.Valid {
		return errEmailAlreadySent
	}

	tx, err := d.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if err := tx.Rollback(); err != nil {
				logrus.Errorf("Error rolling back sendEmail, err: %+v", err)
			}
			return
		}
		err = tx.Commit()
	}()

	collab.EmailSentAt = pq.NullTime{time.Now(), true}
	_, err = tx.Exec(`UPDATE container_collabs SET email_sent_at = $1 WHERE id = $2;`, collab.EmailSentAt, collab.ID)
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
		Fragment: "/dashboard/shared-projects",
	}

	tmplData := struct {
		Subject       string
		Name          string
		SharerName    string
		ContainerSlug string
		ViewUrl       string
	}{
		"",
		recv.DisplayName(),
		template.HTMLEscapeString(sharer.DisplayName()),
		c.Slug,
		url.String(),
	}

	tmplData.Subject = fmt.Sprintf("%s has shared container %s with you!", tmplData.SharerName, tmplData.ContainerSlug)

	htmlBuf := bytes.NewBuffer(nil)
	if err = addedHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err = addedTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	from := envvars.SupportEmail
	to := (&mail.Address{Name: recv.DisplayName(), Address: recv.Email}).String()

	err = Mailer.SendMail(from, []string{to}, []string{from}, nil, from, tmplData.Subject, txtBuf.String(), htmlBuf.String())
	return err
}
