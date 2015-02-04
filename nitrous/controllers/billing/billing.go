package billing

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func CurrentUsage(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	start := now.BeginningOfMonth()
	end := time.Now()
	hours, err := hosttransition.BillableHostHours(env.DB, env.User.ID, start, end)

	if err != nil {
		return err
	}

	availableCredits, err := credit.AvailableCredits(env.DB, env.User.ID)
	if err != nil {
		return err
	}
	availableCreditsDollars := float64(availableCredits) / 100.0

	if len(hours) == 0 {
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"items":                     []interface{}{},
			"total_charge":              0,
			"total_charge_dollars":      0.0,
			"available_credits_dollars": availableCreditsDollars,
		}
		return nil
	}

	total, items := biller.InvoiceItemsForUsages(hours, nil)
	hMap, err := fetchHosts(env.DB, env.User)
	if err != nil {
		return err
	}

	sItems := []serializers.InvoiceItem{}
	for _, inv := range items {
		ser := serializers.SerializeInvoiceItem(&inv)
		ser.Description = invoiceitem.ItemDescription(&inv, hMap[ser.HostID])
		ser.HostSlug = hMap[ser.HostID].Slug
		sItems = append(sItems, *ser)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"items":                     sItems,
		"total_charge":              total,
		"total_charge_dollars":      (float64(total) / 100.0),
		"available_credits_dollars": availableCreditsDollars,
	}

	return nil
}

func Invoices(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	invoices, err := fetchRecentInvoices(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	if len(invoices) == 0 {
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"invoices": []interface{}{},
		}
		return nil
	}

	invIDs := []string{}
	serInvs := []*serializers.Invoice{}
	for _, inv := range invoices {
		invIDs = append(invIDs, strconv.FormatInt(inv.ID, 10))
		sInv := serializers.SerializeInvoice(&inv)
		serInvs = append(serInvs, sInv)
	}

	invItems, err := fetchInvoiceItems(env.DB, invIDs)
	if err != nil {
		return err
	}

	hMap, err := fetchHosts(env.DB, env.User)
	if err != nil {
		return err
	}

	for _, inv := range serInvs {
		for _, item := range invItems {
			if item.InvoiceID == inv.ID {
				if item.Credit.Valid {
					inv.TotalCredit += item.Credit.Int64
				} else {
					ser := serializers.SerializeInvoiceItem(&item)
					ser.Description = invoiceitem.ItemDescription(&item, hMap[ser.HostID])
					if hst, ok := hMap[ser.HostID]; ok {
						ser.HostSlug = hst.Slug
					}
					inv.Items = append(inv.Items, *ser)
				}
			}
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"invoices": serInvs,
	}
	return nil
}

func fetchHosts(db *db.Database, user *user.User) (map[int64]*host.Host, error) {
	hosts, err := host.FindAllWithTerminated(db, user)
	if err != nil {
		return nil, err
	}
	hMap := make(map[int64]*host.Host)
	for indx, host := range hosts {
		hMap[host.ID] = &hosts[indx]
	}
	return hMap, nil
}

func fetchRecentInvoices(d db.QueryReceiver, userID int64) ([]invoice.Invoice, error) {
	invoices := []invoice.Invoice{}
	err := d.Select(&invoices, "SELECT * FROM invoices WHERE user_id = $1 ORDER BY id DESC LIMIT 6;", userID)
	if err != nil {
		return nil, err
	}
	return invoices, nil
}

func fetchInvoiceItems(d db.QueryReceiver, invoiceIDs []string) ([]invoiceitem.InvoiceItem, error) {
	items := []invoiceitem.InvoiceItem{}
	sql := fmt.Sprintf("SELECT * FROM invoice_items WHERE invoice_id IN (%s) ORDER BY id DESC;", strings.Join(invoiceIDs, ","))
	err := d.Select(&items, sql)
	if err != nil {
		return nil, err
	}
	return items, nil
}
