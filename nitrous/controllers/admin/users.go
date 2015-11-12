package admin

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	stripe "github.com/stripe/stripe-go"
	stripesub "github.com/stripe/stripe-go/sub"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errHostCreateInProgress = httperror.New(http.StatusConflict, "host_creation_in_progress", "you cannot delete the account while a workspace is being created")
)

var (
	terminateFunc = hostcontrol.Terminate
)

var (
	ShowUser = VerifyAdmin(showUser)
	BanUser  = VerifyAdmin(banUser)
)

func showUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := mux.Vars(r)["username"]
	users, err := findUsers(db, login)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}
	if len(users) == 0 {
		return errUserNotFound
	}

	serUsers := make([]*serializers.AdminUser, len(users))

	for indx, u := range users {
		up, err := userprofile.Find(db, u.ID)
		if err != nil {
			return err
		}

		bp, err := billingprofile.Find(db, u.ID)
		if err != nil {
			return err
		}

		ser := serializers.AdminSerializeUserWithProfiles(u, up, bp)

		if u.BannedByID.Valid {
			err := db.Get(&ser.BannedBy, "SELECT username FROM users WHERE id = $1", u.BannedByID.Int64)
			if err != nil {
				return err
			}
		}
		serUsers[indx] = ser
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"users": serUsers,
	}
	return nil
}

func banUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) (err error) {
	db := env.DB

	id := mux.Vars(r)["id"]
	uID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}

	u, err := findUser(db, uID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	tx, err := env.DB.Beginx()

	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()

	locked, err := ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(u.ID))
	if err != nil {
		return err
	}

	if !locked {
		return errHostCreateInProgress
	}

	hosts, err := host.FindAllByUser(db, u.ID)
	if err != nil {
		return err
	}

	hdrs := http.Header{}
	hdrs.Set(constants.NitrousUUIDHeader, env.UUID)

	for _, h := range hosts {
		err = env.DB.WithTransaction(func(tx *sqlx.Tx) error {
			return terminateFunc(tx, h, hdrs, env.User.ID, h.State)
		})
		if err != nil {
			return err
		}
	}

	bp, err := u.BillingProfile(db)
	if err != nil {
		return err
	}
	subs, err := subscription.FindAllActiveByUserID(db, u.ID)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		err := stripesub.Cancel(
			sub.StripeID.String,
			&stripe.SubParams{Customer: bp.StripeID.String},
		)
		if err != nil {
			logrus.WithField("uuid", env.UUID).Errorf("Error cancelling subscription %d, err: %v", sub.ID, err)
		}
	}

	queries := []string{
		"DELETE FROM tokens WHERE user_id = $1",
		"DELETE FROM container_collabs WHERE user_id = $1",
		"DELETE FROM verifications WHERE user_id = $1;",
	}

	for _, query := range queries {
		_, err = tx.Exec(query, u.ID)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec("UPDATE users SET banned_by_id = $1, banned_at = now(), deleted_at = now() WHERE id = $2", env.User.ID, u.ID)
	if err != nil {
		return err
	}

	env.Tracker.SetProfileProps("set", u.ID, map[string]interface{}{
		"deleted_date": time.Now(),
		"banned_date":  time.Now(),
	})
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func findUsers(d ndb.QueryReceiver, login string) ([]*user.User, error) {
	users := []*user.User{}
	err := d.Select(&users, "SELECT * FROM users WHERE (username = $1 OR email = $1) ORDER BY deleted_at DESC", login)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func findUser(d ndb.QueryReceiver, id int64) (*user.User, error) {
	user := &user.User{}
	err := d.Get(user, "SELECT * FROM users WHERE id = $1;", id)
	if err != nil {
		return nil, err
	}
	return user, nil
}
