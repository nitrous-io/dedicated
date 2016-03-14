package users

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/emailcapture"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/logintoken"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	validations = map[string]error{
		"index_users_on_username": httperror.New(http.StatusBadRequest, "validation_failed", "username is taken"),
		"index_users_on_email":    httperror.New(http.StatusBadRequest, "validation_failed", "email is taken"),
	}
	reconfirmInvalidUser       = httperror.New(http.StatusBadRequest, "validation_failed", "specified user does not exist")
	errUpdateForbidden         = httperror.New(http.StatusForbidden, "update_forbidden", "You do not have permission to change user settings")
	errExistsForbidden         = httperror.New(http.StatusForbidden, "forbidden", "forbidden")
	errUserNotFound            = httperror.New(http.StatusNotFound, "user_not_found", "not found")
	errExistingPasswordNoMatch = httperror.New(http.StatusBadRequest, "incorrect_existing_password", "your existing passsword is incorrect")
	errInvalidBonusAction      = httperror.New(http.StatusNotFound, "bonus_not_found", "the bonus type is invalid")
	errBonusAlreadyEarned      = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
)

func EmailCapture(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// success no matter what
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	email := r.FormValue("email")
	// mock user to make use of simulated Email checks
	u := &user.User{
		Email: email,
	}
	validation := u.Validate()
	if validation == user.ErrEmailBlank || validation == user.ErrEmailInvalid {
		return nil
	}

	rec := &emailcapture.EmailCapture{
		Email: email,
	}
	err := rec.Insert(env.DB)
	if err != nil {
		logrus.Errorf("Error recording '%s', err: %v", email, err)
	}

	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	if !permission.ProfileSettings(env.Token, u) {
		return errUpdateForbidden
	}

	email, password, name := r.PostFormValue("email"), r.PostFormValue("password"), r.PostFormValue("name")
	existingPassword := r.PostFormValue("existing_password")

	if email == "" && password == "" && name == "" {
		return setUserResponse(env, u)
	}

	if existingPassword != "" && password != "" && !bcrypt.Verify(u.EncryptedPassword, existingPassword) {
		return errExistingPasswordNoMatch
	}

	emailChanged := false
	noUnconfirmedWithSameEmail := !(u.UnconfirmedEmail.String == "" && email == u.Email)
	noSameUnconfirmedEmail := email != u.UnconfirmedEmail.String
	if email != "" && noUnconfirmedWithSameEmail && noSameUnconfirmedEmail {
		if email == u.Email && u.UnconfirmedEmail.String != "" {
			u.UnconfirmedEmail.String = ""
			u.ConfirmationSentAt = pq.NullTime{time.Now(), false}
			u.ConfirmationToken = sql.NullString{"", false}
		} else {
			// verify email
			oU, err := user.FindByUsernameOrEmail(db, email)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if oU != nil && oU.ID != u.ID {
				return validations["index_users_on_email"]
			}

			emailChanged = true
			u.UnconfirmedEmail.String = email
			u.UnconfirmedEmail.Valid = true
		}
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

	if password != "" {
		_, err := db.Exec(`DELETE FROM tokens WHERE user_id = $1 AND id != $2;`, u.ID, env.Token.ID)
		if err != nil {
			return err
		}
	}

	if emailChanged {
		if err := u.SendConfirmationEmail(db, false); err != nil {
			return err
		}
	}

	env.Tracker.SetProfileProps("set", u.ID, map[string]interface{}{
		"$email": u.Email,
		"$name":  u.Name,
	})

	return setUserResponse(env, u)
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

	secondsUsed, err := u.FreeHostUsage(db, time.Now())
	if err != nil {
		return err
	}
	usageRemaining := int64(0)
	if u.UsageQuota.Valid {
		usageRemaining = u.UsageQuota.Int64 - secondsUsed
		if usageRemaining < 0 {
			usageRemaining = 0
		}
	}

	serializedUser := serializers.SerializeUserWithProfiles(u, up, bp)
	serializedUser.UsageRemaining = usageRemaining

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializedUser,
	}
	return nil
}

func GetLoginToken(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}

	token, err := logintoken.Generate(u.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"login_token": token,
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
		if err == sql.ErrNoRows {
			return httperror.New(http.StatusBadRequest, "validation_failed", "specified user does not exist")
		}
		return err
	}

	if err := u.ConfirmEmail(db); err != nil {
		if err == user.ErrConfirmationLapsed {
			return httperror.New(http.StatusBadRequest, "confirmation_expired", err.Error())
		} else if err == user.ErrConfirmationAlreadyCompleted {
			return httperror.New(http.StatusBadRequest, "already_confirmed", err.Error())
		}
		return err
	}

	// Update user's billing profile state to "free" so that user becomes
	// eligible for free plans.
	bp, err := u.BillingProfile(db)
	if err != nil {
		return err
	}
	if bp.State == billingprofile.StateNoCard {
		bp.State = billingprofile.StateFree
		if err := bp.Update(db); err != nil {
			return err
		}
	}

	if u.ConfirmedAt.Valid {
		env.Tracker.SetProfileProps("setnx", u.ID, map[string]interface{}{
			"confirmed_email_date": u.ConfirmedAt.Time,
		})
		env.Tracker.SetProfileProps("set", u.ID, map[string]interface{}{
			"$email": u.Email,
		})
	}

	notifySharedContainers(env, u)

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
			if err == sql.ErrNoRows {
				return reconfirmInvalidUser
			}
			return err
		}
	}

	if appToken != "" {
		t, err := token.FindByToken(db, appToken)
		if err != nil {
			if err == token.ErrInvalidToken {
				return reconfirmInvalidUser
			}
			return err
		}

		u, err = user.FindByID(db, t.UserID)
		if err != nil {
			if err == sql.ErrNoRows {
				return reconfirmInvalidUser
			}
			return err
		}
	}

	if err := u.SendConfirmationEmail(db, false); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"confirmation_sent": true,
	}
	return nil
}

func DismissFirstTime(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}

	_, err := db.Exec(`UPDATE users SET first_time = FALSE WHERE id = $1`, u.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func Exists(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	client, err := getOAuthClient(env, r)
	if err != nil {
		return err
	}
	if client == nil {
		return errExistsForbidden
	}

	email := r.FormValue("email")
	_, err = user.FindByUsernameOrEmail(env.DB, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	env.Status = http.StatusOK
	// since we are using `FindByUsernameOrEmail`, we should explicitly return
	// the parameter given in the form instead of the db record's email column,
	// otherwise consumers of this endpoint can find out what any given
	// username's e-mail address is
	env.Data = map[string]interface{}{
		"found": email,
	}
	return nil
}

func setUserResponse(env *middleware.Env, u *user.User) error {
	up, err := userprofile.Find(env.DB, u.ID)
	if err != nil {
		return err
	}

	bp, err := billingprofile.Find(env.DB, u.ID)
	if err != nil {
		return err
	}

	secondsUsed, err := u.FreeHostUsage(env.DB, time.Now())
	if err != nil {
		return err
	}
	usageRemaining := int64(0)
	if u.UsageQuota.Valid {
		usageRemaining = u.UsageQuota.Int64 - secondsUsed
		if usageRemaining < 0 {
			usageRemaining = 0
		}
	}

	serializedUser := serializers.SerializeUserWithProfiles(u, up, bp)
	serializedUser.UsageRemaining = usageRemaining

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializedUser,
	}
	return nil
}

func notifySharedContainers(env *middleware.Env, u *user.User) {
	cns, err := container.FindAllShared(env.DB, u.ID)
	if err != nil {
		logrus.Errorf("Unable to fetch containers, err: %v", err)
	}
	for _, cn := range cns {
		containers.PublishContainerEvent(env, controllers.EventCollaboratorAdded, cn, nil)
	}
}
