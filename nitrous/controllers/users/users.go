package users

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/emailcapture"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/verification"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/subosito/twilio"
)

var (
	validations = map[string]error{
		"index_users_on_username":                     httperror.New(http.StatusBadRequest, "validation_failed", "username is taken"),
		"index_users_on_email":                        httperror.New(http.StatusBadRequest, "validation_failed", "email is taken"),
		"index_users_on_phone_and_phone_country_code": httperror.New(http.StatusBadRequest, "validation_failed", "phone number already exists"),
	}
	reconfirmInvalidUser = httperror.New(http.StatusBadRequest, "validation_failed", "specified user does not exist")
	invalidPromoCode     = httperror.New(http.StatusBadRequest, "invalid_promo_code", "promo code does not exist")
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
		env.Logger.Errorf("Error recording '%s', err: %v", email, err)
	}

	return nil
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	email := strings.ToLower(r.FormValue("email"))
	username := strings.ToLower(r.FormValue("username"))
	password := r.FormValue("password")
	name := r.FormValue("name")
	promocode := r.FormValue("promo_code")

	u := &user.User{
		Email:     email,
		Username:  username,
		Name:      name,
		Password:  password,
		PromoCode: sql.NullString{promocode, true},
	}

	err := u.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	if promocode != "" {
		if _, err := credittype.FindValidCreditTypeByName(tx, promocode); err != nil {
			tx.Rollback()
			if err == sql.ErrNoRows {
				return invalidPromoCode
			}
			return err
		}
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

	userResponse(env, u)
	return nil
}

func UpdatePhone(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User

	phone := sanitizePhone(r.PostFormValue("phone"))
	phoneCountryCode := r.PostFormValue("phone_country_code")
	if phone == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "phone must be provided")
	}
	if phoneCountryCode == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "phone country code must be provided")
	}

	if err := u.UpdatePhone(env.DB, phone, phoneCountryCode); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	if err := sendVerificationCode(env.DB, env.Logger, u); err != nil {
		return err
	}

	userResponse(env, u)
	return nil
}

func ResendPhoneCode(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if err := sendVerificationCode(env.DB, env.Logger, env.User); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"sent": true,
	}
	return nil
}

func sendVerificationCode(d *db.Database, l *logger.Logger, u *user.User) error {
	if !u.Phone.Valid || !u.PhoneCountryCode.Valid {
		return httperror.New(http.StatusBadRequest, "invalid_request",
			"we do not have your phone number in our records")
	}

	// Send verification code to user via SMS to verify he's a real person™.
	v, err := verification.Generate(d, u.ID, u.PhoneNumber())
	if err != nil {
		return err
	}

	client := twilio.NewClient(envvars.TwilioAccountSID, envvars.TwilioAuthToken, nil)
	msg := fmt.Sprintf("Your Nitrous verification code is: %s", v.Code)
	m, _, err := client.Messages.SendSMS(envvars.TwilioNumber, u.PhoneNumber(), msg)
	if err != nil {
		errlog := fmt.Sprintf("user.UpdatePhone: failed to send SMS, message: %+v, err: %+v", m, err)
		logAsErr, msg := twilioError(err)
		if logAsErr {
			l.Errorf(errlog)
		} else {
			l.Debugf(errlog)
		}
		return httperror.New(http.StatusServiceUnavailable, "sms_failed", msg)
	}

	return nil
}

func inWhitelist(code string) bool {
	for _, c := range PhoneCountryWhitelist {
		if c == code {
			return true
		}
	}
	return false
}

func VerifyPhone(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	d := env.DB

	code := r.PostFormValue("code")
	if code == "" {
		return httperror.New(http.StatusBadRequest, "validation_failed", "code must be provided")
	}
	if !u.Phone.Valid || !u.PhoneCountryCode.Valid {
		return httperror.New(http.StatusBadRequest, "invalid_request",
			"we do not have your phone number in our records")
	}

	v, err := verification.Find(d, u.ID, u.PhoneNumber())
	if err != nil {
		return err
	}
	if v.Code != code {
		return httperror.New(http.StatusBadRequest, "validation_failed", "code is incorrect")
	}

	if err := u.UpdatePhoneAuthed(d, true); err != nil {
		return err
	}

	// If user's phone country is in whitelist, we set user's billing profile to
	// free so that user is eligible for the free tier plan.
	bp, err := billingprofile.Find(d, u.ID)
	if err != nil {
		return err
	}
	if bp.State == billingprofile.StateNoCard &&
		inWhitelist(u.PhoneCountryCode.String) {
		bp.State = billingprofile.StateFree
		err := bp.Update(d)
		if err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"verified": true,
	}
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

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUserWithProfiles(u, up, bp),
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

	if err = u.ConfirmEmail(db); err != nil {
		if err == user.ErrConfirmationLapsed {
			return httperror.New(http.StatusBadRequest, "confirmation_expired", err.Error())
		} else if err == user.ErrConfirmationAlreadyCompleted {
			return httperror.New(http.StatusBadRequest, "already_confirmed", err.Error())
		}
		return err
	}
	if u.ConfirmedAt.Valid {
		env.Tracker.SetProfileProps("setnx", u.ID, map[string]interface{}{
			"confirmed_email_date": u.ConfirmedAt.Time,
		}, env.Logger)
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

	err = u.SendConfirmationEmail(db, false)
	if err != nil {
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

func userResponse(env *middleware.Env, u *user.User) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	return
}

func sanitizePhone(phoneNum string) string {
	onlyNumeric := func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}
	return strings.Map(onlyNumeric, phoneNum)
}

// twilioError parses an error and returns if it should be treated as error, and a user-facing message https://github.com/subosito/twilio/blob/ca29365f195cd8e9dfb805d4b8aca69ce8057564/exception.go
func twilioError(e error) (bool, string) {
	defaultMsg := "there was an error sending the verification code to your phone"
	terr, ok := e.(*twilio.Exception)
	if !ok {
		return true, defaultMsg
	}
	switch terr.Code {
	case 21211: // https://www.twilio.com/docs/errors/21211
		return false, fmt.Sprintf("%s: number is invalid", defaultMsg)
	case 21614: // https://www.twilio.com/docs/errors/21614
		return false, fmt.Sprintf("%s: number cannot receive SMS", defaultMsg)
	default:
		return true, defaultMsg
	}

}
