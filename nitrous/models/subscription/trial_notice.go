package subscription

import (
	"bytes"
	"fmt"
	"net/mail"
	"net/url"
	"text/template"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

var (
	Mailer mailer.Mailer

	trialEmailHtmlTemplate *template.Template
	trialEmailTxtTemplate  *template.Template
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	trialEmailHtmlTemplate =
		template.Must(template.New("trialEmailHtml").Parse(trialEmailHtmlDef))

	trialEmailTxtTemplate =
		template.Must(template.New("trialEmailTxt").Parse(trialEmailTxtDef))
}

func SendTrialEndingEmail(dbConn *db.Database, s *SubscriptionWithUserInfo, dryrun bool) error {
	logrus.Infof("Sending trial ending email to user %v for subscription %d", s.UserID, s.ID)

	u, err := user.FindByUsername(dbConn, s.Username)
	if err != nil {
		return fmt.Errorf("Error getting user %s by username: %v", s.Username, err)
	}

	subject := fmt.Sprintf("Free Trial Expiring")
	from := envvars.SupportEmail
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()
	htmlBuf, txtBuf, err := composeTmpl(dbConn, s, u)
	if err != nil {
		return err
	}

	if !dryrun {
		if err := Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, txtBuf.String(), htmlBuf.String()); err != nil {
			return fmt.Errorf("Mailer.SendMail: %v", err)
		}
	} else {
		logrus.Debugf("Sending from %s to %s, subject %s, txtBuf: %s", from, to, subject, txtBuf.String())
	}

	return nil
}

func composeTmpl(dbConn *db.Database, s *SubscriptionWithUserInfo, u *user.User) (*bytes.Buffer, *bytes.Buffer, error) {
	bURL := &url.URL{
		Scheme:   "https",
		Host:     "www.nitrous.io",
		Path:     "/app/",
		Fragment: "/billing",
	}

	plan, err := plan.FindByID(dbConn, s.PlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("Error getting plan by ID: %v", s.PlanID, err)
	}

	tmplData := struct {
		Name, TrialEnd, Plan, BillingsURL string
	}{
		u.DisplayName(),
		s.TrialEnd.Time.Format("Monday Jan _2 15:04 2006"),
		plan.Name,
		bURL.String(),
	}

	htmlBuf := bytes.NewBuffer(nil)
	if err := trialEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return nil, nil, fmt.Errorf("trialEmailHtmlTemplate.Execute: %v", err)
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := trialEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return nil, nil, fmt.Errorf("trialEmailTxtTemplate.Execute: %v", err)
	}

	return htmlBuf, txtBuf, nil
}

const trialEmailHtmlDef = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <title>Free Trial Expiring</title>
  </head>
  <body>
    <h3>Hello {{ .Name }}!</h3>
    <p>Thanks for trying our free trial!</p>
    <p>We note that your trial is ending on <b>{{ .TrialEnd }}</b>. Thereafter, your monthly subscription for our {{ .Plan }} plan will begin and you will be billed on a monthly basis.</p>

    <p>We hope you enjoyed using Nitrous. If you do not wish to subscribe to the paid plan, you can log on to {{ .BillingsURL }} to cancel your subscription before your trial expires.</p>

    <p>Start your engines,</p>
    <p>The Nitrous team</p>
  </body>
</html>
`

const trialEmailTxtDef = `
Hello {{ .Name }}!

Thanks for trying our free trial!

We note that your trial is ending on {{ .TrialEnd }}. Thereafter, your monthly subscription for our {{ .Plan }} plan will begin and you will be billed on a monthly basis.

We hope you enjoyed using Nitrous. If you do not wish to subscribe to the paid plan, you can log on to {{ .BillingsURL }} to cancel your subscription before your trial expires.

Start your engines,
The Nitrous team
`
