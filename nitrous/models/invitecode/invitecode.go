package invitecode

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

const INVITE_CODE_LENGTH = 32

type InviteCode struct {
	ID   int64  `db:"id"`
	Code string `db:"code"`
	Used bool   `db:"used"`
}

func FindByCode(d db.QueryReceiver, code string) (*InviteCode, error) {
	c := &InviteCode{}
	err := d.Get(c, "SELECT * FROM invite_codes WHERE code = $1", code)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func MarkAsUsed(d db.QueryReceiver, code string) error {
	_, err := d.Exec("UPDATE invite_codes SET used = true WHERE code = $1", code)
	return err
}

func GenerateInvite(d db.QueryReceiver) (*InviteCode, error) {
	c, err := securerandom.Base64(INVITE_CODE_LENGTH)
	if err != nil {
		return nil, err
	}
	code := &InviteCode{
		Code: c[:INVITE_CODE_LENGTH],
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO invite_codes (code) VALUES (:code) RETURNING *;`)
	if err != nil {
		return nil, err
	}

	if err = nstmt.QueryRowx(code).StructScan(code); err != nil {
		return nil, err
	}
	return code, nil
}
