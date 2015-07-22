package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/mail"
	"os"
	"text/template"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

const Usage = `Usage: arbimail <subject> <recipients csv file> <html template> <text template> [--live-run]
  <file> is a 2-column csv file of id,email
  Subject is template text
  The templates are text files containing Go templates that are sent
  models/user.User structs.
  Use --live-run to actually send e-mails, otherwise it will be a dry run
`

var (
	dbConn *db.Database
	dryrun = true

	subjTmpl *template.Template
	htmlTmpl *template.Template
	textTmpl *template.Template
	sgMailer mailer.Mailer
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	requiredArgs := 4
	for _, arg := range os.Args {
		if arg == "--live-run" {
			dryrun = false
			requiredArgs += 1
		}
	}
	if len(os.Args) != requiredArgs+1 {
		fmt.Println(Usage)
		logrus.Fatal("Wrong number of arguments")
	}

	subjectTmplText, csvPath, htmlPath, textPath := os.Args[1], os.Args[2], os.Args[3], os.Args[4]

	envvars.CheckDB()
	envvars.CheckSendgrid()
	dbURL := envvars.NitrousPgUrl
	sgMailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatal(err)
	}

	rows, err := parseCSV(csvPath)
	if err != nil {
		logrus.Fatalf("Error parsing csv: %s", err)
	}

	htmlTmpl, textTmpl, err = parseTemplates(htmlPath, textPath)
	if err != nil {
		logrus.Fatalf("Error parsing templates: %s", err)
	}

	subjTmpl, err = template.New("subject").Parse(subjectTmplText)
	if err != nil {
		logrus.Fatalf("Error parsing subject: %s", err)
	}

	for _, row := range rows {
		renderAndSend(row[1])
	}

}

func renderAndSend(email string) error {
	u, err := user.FindByUsernameOrEmail(dbConn, email)
	if err != nil {
		return err
	}
	if u == nil {
		return fmt.Errorf("User %s not found", email)
	}
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()
	logrus.Debugf("Rendering for user: %s", to)

	subjBuf, htmlBuf, textBuf := bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	if err := htmlTmpl.Execute(htmlBuf, u); err != nil {
		return err
	}
	if err := textTmpl.Execute(textBuf, u); err != nil {
		return err
	}
	if err := subjTmpl.Execute(subjBuf, u); err != nil {
		return err
	}

	if dryrun {
		fmt.Println("Subject: ", subjBuf.String())
		fmt.Println("HTML template:")
		fmt.Println("=== START ===")
		fmt.Println(htmlBuf.String())
		fmt.Println("===  END  ===\n")

		fmt.Println("Text template:\n")
		fmt.Println("=== START ===")
		fmt.Println(textBuf.String())
		fmt.Println("===  END  ===\n")

		return nil
	}

	from := (&mail.Address{Name: "Nitrous Pro", Address: "support@nitrous.io"}).String()

	err = sgMailer.SendMail(from, []string{to}, []string{from}, nil, from,
		subjBuf.String(), textBuf.String(), htmlBuf.String())
	if err != nil {
		logrus.Fatalf("Error sending: %s", err)
		time.Sleep(500 * time.Millisecond)
	}
	logrus.Debugf("Sent to user: %s", to)
	return nil
}

func parseCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 2
	if _, err := r.Read(); err != nil { // headers
		return nil, err
	}
	return r.ReadAll()
}

func parseTemplates(htmlPath, textPath string) (*template.Template, *template.Template, error) {
	h, err := template.ParseFiles(htmlPath)
	if err != nil {
		return nil, nil, err
	}
	t, err := template.ParseFiles(textPath)
	if err != nil {
		return nil, nil, err
	}
	return h, t, nil
}
