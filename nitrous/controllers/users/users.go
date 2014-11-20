package users

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/invitecode"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	validations = map[string]error{
		"index_users_on_username": httperror.New(http.StatusBadRequest, "validation_failed", "username is taken"),
		"index_users_on_email":    httperror.New(http.StatusBadRequest, "validation_failed", "email is taken"),
	}
	reconfirmInvalidUser = httperror.New(http.StatusBadRequest, "validation_failed", "specified user does not exist")
)

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	code := r.FormValue("code")
	var ic *invitecode.InviteCode
	var err error
	if code == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "code can't be blank")
	} else {
		ic, err = invitecode.FindByCode(db, code)
		if err != nil {
			return err
		}
		if ic == nil {
			return httperror.New(http.StatusBadRequest, "validation_failed", "code does not exist")
		}
	}

	email := strings.ToLower(r.FormValue("email"))
	username := strings.ToLower(r.FormValue("username"))
	password := r.FormValue("password")
	name := r.FormValue("name")

	u := &user.User{
		Email:        email,
		Username:     username,
		Name:         name,
		Password:     password,
		InviteCodeID: sql.NullInt64{ic.ID, true},
	}

	err = u.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = u.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	err = invitecode.MarkAsUsed(tx, ic.Code)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	err = u.SendConfirmationEmail(db, true)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	env.User = u

	env.Tracker.CreateProfile(u, env.Logger, code)
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	email, password, name := r.PostFormValue("email"), r.PostFormValue("password"), r.PostFormValue("name")

	if email == "" && password == "" && name == "" {
		userResponse(env, u)
		return nil
	}

	emailChanged := false
	if email != "" {
		emailChanged = u.Email != email
		u.Email = email
	}
	if password != "" {
		u.Password = password
	}
	if name != "" {
		u.Name = name
	}

	if err := u.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	if err := u.Update(db); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	if emailChanged {
		u.ConfirmedAt = nil
		if err := u.SendConfirmationEmail(db, true); err != nil {
			return err
		}
	}

	userResponse(env, u)
	return nil
}

func GetAuthenticatedUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}

	up, err := userprofile.Find(db, u.ID)
	if err != nil {
		return err
	}

	bp, err := billingprofile.Find(db, u.ID)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeUser(u)
	serialized.UserProfile = serializers.SerializeUserProfile(up)
	serialized.BillingProfile = serializers.SerializeBillingProfile(bp)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serialized,
	}
	return nil
}

func Confirm(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	token := r.FormValue("token")
	if token == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "`token` must be provided")
	}

	u, err := user.FindByConfirmationToken(db, token)
	if err != nil {
		return err
	}

	if u == nil || token != u.ConfirmationToken.String {
		return httperror.New(http.StatusBadRequest, "validation_failed", "specified user does not exist")
	}

	err = u.ConfirmEmail(db)
	if err != nil {
		if err == user.ErrConfirmationLapsed {
			return httperror.New(http.StatusBadRequest, "confirmation_expired", err.Error())
		} else if err == user.ErrConfirmationAlreadyCompleted {
			return httperror.New(http.StatusBadRequest, "already_confirmed", err.Error())
		}
		return err
	}

	return env.Login(u, w)
}

func ResendConfirmation(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cToken := r.FormValue("token")
	appToken := r.FormValue("app_token")
	if cToken == "" && appToken == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "`token` must be provided")
	}

	var u *user.User
	var err error

	if cToken != "" {
		u, err = user.FindByConfirmationToken(db, cToken)
		if err != nil {
			return err
		}
	}

	if appToken != "" {
		t, err := token.FindByToken(db, appToken)
		if err != nil {
			return err
		}
		if t == nil {
			return reconfirmInvalidUser
		}

		u, err = user.FindByID(db, t.UserID)
		if err != nil {
			return err
		}
	}

	if u == nil {
		return reconfirmInvalidUser
	}

	err = u.SendConfirmationEmail(db, false)
	if err != nil {
		if err == user.ErrConfirmationAlreadyCompleted {
			return httperror.New(http.StatusBadRequest, "already_confirmed", err.Error())
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"confirmation_sent": true,
	}
	return nil
}

func userResponse(env *middleware.Env, u *user.User) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	return
}
