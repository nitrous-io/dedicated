package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/referralcode"
)

type ReferralCode struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"user_id"` // Referer's user id
	Code   string `json:"code"`

	Bonus int64 `json:"bonus"`
	Cap   int64 `json:"cap"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeReferralCode(rc *referralcode.ReferralCode) *ReferralCode {
	if rc == nil {
		return nil
	}
	return &ReferralCode{
		ID:        rc.ID,
		UserID:    rc.UserID,
		Code:      rc.Code,
		Bonus:     rc.Bonus,
		Cap:       rc.Cap,
		CreatedAt: rc.CreatedAt,
		UpdatedAt: rc.UpdatedAt,
	}
}
