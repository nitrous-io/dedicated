package subscriptions

import (
	"bytes"
	"fmt"
	"net/mail"
	"net/url"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func (s *SubscriptionProvider) sendFailureEmail(d db.QueryReceiver, tryCount int64, final bool, inv *invoice.Invoice, bp *billingprofile.BillingProfile) error {
	// TODO: when Organisations are introduced, use org or user for email contact
	u, err := user.FindByID(d, bp.UserID)
	if err != nil {
		return err
	}

	from := envvars.SupportEmailAddr
	to := &mail.Address{Name: u.DisplayName(), Address: u.Email}

	tmplData := struct {
		Name          string
		LastFour      string
		InvoiceAmount string
		SupportEmail  string
		BillingUrl    string
	}{
		u.DisplayName(),
		"unspecified",
		fmt.Sprintf("$%.2f", float64(inv.TotalCharge)/100.0),
		from.Address,
		billingUrl(),
	}
	if tmplData.Name == "" {
		tmplData.Name = u.Username
	}

	if bp.LastFour.String != "" {
		tmplData.LastFour = bp.LastFour.String
	}

	htmlTemplate := paymentFirstFailedHtmlTemplate
	txtTemplate := paymentFirstFailedTxtTemplate
	subject := "Nitrous - payment unsuccessful"
	switch {
	case final:
		htmlTemplate = paymentFinalFailedHtmlTemplate
		txtTemplate = paymentFinalFailedTxtTemplate
		subject = "Nitrous - your workspaces will be terminated"
	case tryCount == 2:
		htmlTemplate = paymentSecondFailedHtmlTemplate
		txtTemplate = paymentSecondFailedTxtTemplate
		subject = "Nitrous - payment retry unsuccessful"
	case tryCount == 3:
		htmlTemplate = paymentThirdFailedHtmlTemplate
		txtTemplate = paymentThirdFailedTxtTemplate
		subject = "Nitrous - final warning"
	}

	logrus.Infof("Sending warning email %q for attemptCount %d for user %d", htmlTemplate.Name(), tryCount, u.ID)

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err := htmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := txtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	return s.Mailer.SendMail(from.String(), []string{to.String()}, []string{from.String()}, nil, from.String(), subject, txtBuf.String(), htmlBuf.String())
}

func billingUrl() string {
	host := envvars.WebHostname
	if host == "" {
		host = "pro.nitrous.io"
	}
	u := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app/",
		Fragment: "/billing",
	}
	return u.String()
}
