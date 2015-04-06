package containercollabs

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	Index  middleware.Middleware = containers.WithContainerAndHost(index, true)
	Add    middleware.Middleware = containers.WithContainerAndHost(add, false)
	Remove middleware.Middleware = containers.WithContainerAndHost(remove, false)

	errMissingUserParam = httperror.New(http.StatusBadRequest, "invalid_request", "`user` must be provided")
	errUserNotFound     = httperror.New(http.StatusBadRequest, "user_not_found", "the specified user could not be found")
	errSameUser         = httperror.New(http.StatusBadRequest, "same_user", "you cannot add yourself as a collaborator")
	errAlreadyAdded     = httperror.New(http.StatusConflict, "already_added", "this user is already a collaborator")
	errEmailAlreadySent = errors.New("email already sent")
	Mailer              mailer.Mailer
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

func index(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	sc, err := containercollab.FindAll(env.DB, cn.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"collaborators": serializers.SerializeContainerCollabUsers(sc),
	}
	return nil
}

func add(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	username := r.FormValue("user")
	if username == "" {
		return errMissingUserParam
	}

	usr, err := user.FindByUsernameOrEmail(env.DB, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	if usr.ServiceAccount {
		return errUserNotFound
	}

	if usr.ID == env.User.ID {
		return errSameUser
	}

	collab, err := containercollab.Create(env.DB, cn.ID, usr.ID)
	if err != nil {
		pqErr, ok := err.(*pq.Error)
		if ok && strings.Contains(pqErr.Message, "duplicate key") {
			return errAlreadyAdded
		}
		return err
	}

	go func() {
		err := sendEmail(env.Logger, env.DB, cn, collab, env.User, usr)
		if err != nil {
			env.Logger.Errorf("Failed to send email for collaborator %d, user id: %d err: %+v", collab.ID, env.User.ID, err)
		}
	}()

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

	err = containercollab.Delete(env.DB, ccid)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"success": true,
	}
	return nil
}

func sendEmail(l *logger.Logger, d *db.Database, c *container.Container, collab *containercollab.ContainerCollab, sharer *user.User, recv *user.User) (err error) {
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
				l.Errorf("Error rolling back sendEmail, err: %+v", err)
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
		host = "pro.nitrous.io"
	}
	url := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app/",
		Fragment: fmt.Sprintf("/containers/%s", c.Slug),
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
		sharer.DisplayName(),
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

	from := (&mail.Address{Name: "Nitrous Pro", Address: "mailer@nitrous.io"}).String()
	to := (&mail.Address{Name: recv.DisplayName(), Address: recv.Email}).String()

	err = Mailer.SendMail(from, []string{to}, []string{from}, nil, from, tmplData.Subject, txtBuf.String(), htmlBuf.String())
	return err
}
