package verification

import (
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

type Verification struct {
	ID        int64       `db:"id"`
	UserID    int64       `db:"user_id"`
	Phone     string      `db:"phone"`
	Code      string      `db:"code"`
	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

func Generate(d *db.Database, userID int64, phone string) (*Verification, error) {
	rnd, err := securerandom.Numeric(6)
	if err != nil {
		return nil, err
	}

	v := &Verification{
		UserID: userID,
		Phone:  phone,
		Code:   rnd,
	}
	if err := v.save(d); err != nil {
		return nil, err
	}

	return v, nil
}

func Find(d db.QueryReceiver, userID int64, phone string) (*Verification, error) {
	v := &Verification{}
	err := d.Get(v, `SELECT * FROM verifications WHERE user_id = $1 AND phone = $2;`, userID, phone)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Verification) save(d *db.Database) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete any existing verifications for the user.
	if _, err := tx.Exec(`DELETE FROM verifications WHERE user_id = $1`, v.UserID); err != nil {
		return err
	}

	nstmt, err := tx.PrepareNamed(`INSERT INTO verifications
		(user_id, phone, code, created_at, updated_at)
    VALUES (:user_id, :phone, :code, now(), now())
		RETURNING *;`)
	if err != nil {
		return err
	}
	if err := nstmt.QueryRowx(v).StructScan(v); err != nil {
		return err
	}

	return tx.Commit()
}
