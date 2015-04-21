package freequeue

import (
	"bytes"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	Pending = admin.VerifyAdmin(pending)
	Action  = admin.VerifyAdmin(action)

	errExpectedStateRequired = httperror.New(http.StatusBadRequest, "invalid_request", "`expected_state` is required")
	errNewStateRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "`new_state` is required")
	errInvalidState          = httperror.New(http.StatusBadRequest, "invalid_request", "the provided `new_state` is invalid")
	errInconsistentState     = httperror.New(http.StatusConflict, "inconsistent_state", "the provided `expected_state` does not match the current state")

	Mailer mailer.Mailer
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

func pending(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pending, err := freequeue.FindAllPending(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"queue": serializers.SerializeFreeQueueWithUsers(pending),
	}
	return nil
}

func action(env *middleware.Env, w http.ResponseWriter, r *http.Request) (err error) {
	expected := r.FormValue("expected_state")
	if expected == "" {
		return errExpectedStateRequired
	}

	state := r.FormValue("new_state")
	if state == "" {
		return errNewStateRequired
	}

	id := mux.Vars(r)["id"]
	fqID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
		tx.Commit()
	}()

	fq, err := freequeue.Action(tx, fqID, expected, state, env.User.ID)
	if err != nil {
		if err == freequeue.ErrInvalidState {
			return errInvalidState
		}
		if err == freequeue.ErrInconsistentState {
			return errInconsistentState
		}
		return err
	}

	if fq.State == freequeue.StateApproved {
		if err = sendMail(tx, fq); err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func sendMail(tx *sqlx.Tx, fq *freequeue.FreeQueue) error {
	u, err := user.FindByID(tx, fq.UserID)
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
		Fragment: fmt.Sprintf("/quick-create?template=%s&name=%s&region=%s", fq.ContainerTemplateSlug, fq.ContainerName, fq.RegionSlug),
	}

	tmplData := struct {
		Subject      string
		Name         string
		ActionUrl    string
		SupportEmail string
	}{
		"Your Nitrous workspace is ready!",
		u.DisplayName(),
		url.String(),
		"support@nitrous.io",
	}

	htmlBuf := bytes.NewBuffer(nil)
	if err = approvedHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err = approvedTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	from := (&mail.Address{Name: "Nitrous Pro", Address: "mailer@nitrous.io"}).String()
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()

	return Mailer.SendMail(from, []string{to}, []string{from}, nil, from, tmplData.Subject, txtBuf.String(), htmlBuf.String())
}
