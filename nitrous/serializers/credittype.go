package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
)

type CreditType struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Active       bool       `json:"active"`
	DefaultValue int64      `json:"default_value"` // stored as cents
	Stock        *int64     `json:"stock,omitempty"`
	Used         *int64     `json:"used,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func SerializeCreditType(c *credittype.CreditType) *CreditType {
	if c == nil {
		return nil
	}
	ser := &CreditType{
		ID:           c.ID,
		Name:         c.Name,
		Description:  c.Description.String,
		Active:       c.Active,
		DefaultValue: c.DefaultValue,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
	if c.Stock.Valid {
		ser.Stock = &c.Stock.Int64
	}
	if c.Used.Valid {
		ser.Used = &c.Used.Int64
	}
	if c.ExpiresAt.Valid {
		ser.ExpiresAt = &c.ExpiresAt.Time
	}
	return ser
}

func SerializeCreditTypes(cts []*credittype.CreditType) []*CreditType {
	ret := make([]*CreditType, len(cts))
	for i, p := range cts {
		ret[i] = SerializeCreditType(p)
	}
	return ret
}
