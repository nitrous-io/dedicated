package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	nuser "github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/analytics"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/onsi/gomega"
	bc "golang.org/x/crypto/bcrypt"
)

func init() {
	bcrypt.DefaultCost = bc.MinCost
	logrus.SetLevel(logrus.PanicLevel)
}

var tables []string // retrieve list of tables once and store here

func TestDatabase(dbname string) (*db.Database, error) {
	d, err := ConnectToTestDatabase(dbname)
	if err != nil {
		return nil, err
	}
	if err := d.MigrateAll(); err != nil {
		d.Close()
		return nil, err
	}
	if err := truncateTables(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func ConnectToTestDatabase(dbname string) (*db.Database, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	dburl := fmt.Sprintf("host=/var/run/postgresql user=%s dbname=%s sslmode=disable", u.Username, dbname)
	d, err := db.NewDatabase(dburl)
	if err != nil {
		if d != nil {
			d.Close()
		}
		return nil, err
	}
	return d, nil
}

func truncateTables(db *db.Database) error {
	if len(tables) == 0 {
		tbls, err := getTables(db)
		if err != nil {
			return err
		}
		tables = tbls
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`SET local session_replication_role TO 'replica'`); err != nil {
		tx.Rollback()
		return err
	}

	for _, tb := range tables {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s`, tb)); err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return err
	}

	return nil
}

func getTables(db *db.Database) ([]string, error) {
	rows, err := db.Query(`SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations';`)
	if err != nil {
		return []string{}, err
	}
	defer rows.Close()
	tbls := []string{}
	for rows.Next() {
		var tablename string
		err = rows.Scan(&tablename)
		if err != nil {
			return []string{}, err
		}
		tbls = append(tbls, tablename)
	}
	return tbls, nil
}

func NewBodyRequest(method, urlStr, bodyStr string) (*http.Request, error) {
	return http.NewRequest(method, urlStr, bytes.NewBufferString(bodyStr))
}

func NewAPIRequest(token string, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func NewRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return http.NewRequest(method, u.String(), nil)
}

func NewAuthedRequest(t *token.Token, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return req, nil
}

func NewFormRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	var r io.Reader
	if params != nil {
		r = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequest(method, urlStr, r)
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

func NewAuthedFormRequest(t *token.Token, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewFormRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return req, nil
}

// ErrorSlug expects a JSON object as string with 'error' and
// 'error_description' keys, and returns those respectively.  It swallows any
// errors and returns two empty strings.
func ErrorSlug(body io.Reader) (string, string) {
	j := map[string]string{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		return "", ""
	}
	return j["error"], j["error_description"]
}

func TempFile(extension string) string {
	return path.Join(os.TempDir(), fmt.Sprintf("%v.%v", RandomName("freightertest"), extension))
}

func TempDir() (string, error) {
	dir := path.Join(os.TempDir(), RandomName("freightertest"))
	err := os.Mkdir(dir, 0755)
	if err != nil {
		return "", err
	}
	return dir, nil
}

func RandomName(name string) string {
	return fmt.Sprintf("%v-%v-%v", name, time.Now().UnixNano(), rand.Intn(1000))
}

func WrapInTransaction(db *db.Database, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = fn(tx)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func NewTestServer(root string, db *db.Database, t analytics.Tracker) *server.Server {
	svr := server.NewServer(root, db, nil, nil)
	svr.Tracker = t

	return svr
}

func MarkDelinquentUser(db *db.Database, u *nuser.User) {
	_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE user_id = $2;`, billingprofile.StateDelinquent, u.ID)
	Expect(err).To(BeNil())
}

func MarkDelinquentBP(db *db.Database, bp *billingprofile.BillingProfile) {
	_, err := db.Exec(`UPDATE billing_profiles SET state = $1 WHERE id = $2;`, billingprofile.StateDelinquent, bp.ID)
	Expect(err).To(BeNil())
}

func VerifyDelinquentUserResponse(res *httptest.ResponseRecorder) {
	Expect(res.Code).To(Equal(http.StatusBadRequest))
	err, errSlug := ErrorSlug(res.Body)
	Expect(err).To(Equal("outstanding_invoices"))
	Expect(errSlug).To(Equal("you have outstanding invoices, please check your billing settings"))
}

// FakeRoundTripper implements a fake Transport layer that records the executed
// requests and provides a fake response.
//
// This is more lightweight than implementing a full HTTP server and should be
// appropriate for some tests.
type FakeRoundTripper struct {
	Message  string
	Status   int
	Wait     time.Duration
	Header   map[string]string
	Requests []*http.Request
	Proxy    func(req *http.Request)
}

func (rt *FakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.Wait > 0 {
		time.Sleep(rt.Wait)
	}
	body := strings.NewReader(rt.Message)
	rt.Requests = append(rt.Requests, r)
	res := &http.Response{
		StatusCode: rt.Status,
		Body:       ioutil.NopCloser(body),
		Header:     make(http.Header),
	}
	for k, v := range rt.Header {
		res.Header.Set(k, v)
	}

	if rt.Proxy != nil {
		rt.Proxy(r)
	}
	return res, nil
}

func (rt *FakeRoundTripper) Reset() {
	rt.Requests = nil
}

func (rt *FakeRoundTripper) CancelRequest(req *http.Request) {
	// No-op. Satisfy http.Transport.CancelRequest() that's needed when
	// setting timeout on a http.Client.
}

// listens on msgCh, waits for event with Name `name`, and returns it.  Returns
// error on given timeout.  Where msgCh is typically looks like `msgCh :=
// svr.Events.Subscribe(pubsub.Topic("some topic"))`
func WaitForEvent(msgCh chan pubsub.Message, name controllers.EventName, timeout time.Duration) (*controllers.Event, error) {
	evtCh := make(chan *controllers.Event)
	go func() {
		var evt *controllers.Event
		for msg := range msgCh { // Ignore events we don't care about.
			evt = msg.(*controllers.Event)
			if evt.Name == name {
				evtCh <- evt
				break
			}
		}
	}()

	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("Timeout waiting for event name %s (%s)", name, timeout)
	case evt := <-evtCh:
		Expect(evt.Name).To(Equal(name))
		return evt, nil
	}
}
