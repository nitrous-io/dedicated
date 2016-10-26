package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostkeys"
	"golang.org/x/crypto/ssh"
)

var (
	awsAccessKey      string
	awsSecretKey      string
	s3Bucket          string
	hostbuilderURL    string
	hostbuilderClient *hostbuilderclient.Hostbuilder
)

type job struct {
	user *user.User
	host *host.Host
}

func main() {
	err := run()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	awsAccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	if awsAccessKey == "" {
		log.Fatalf("AWS_ACCESS_KEY_ID is required.")
	}
	awsSecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	if awsSecretKey == "" {
		log.Fatalf("AWS_SECRET_ACCESS_KEY is required.")
	}
	if s3Bucket = os.Getenv("S3_BUCKET_NAME"); s3Bucket == "" {
		log.Fatalf("S3_BUCKET_NAME is required.")
	}
	if hostbuilderURL = os.Getenv("HOSTBUILDER_URL"); hostbuilderURL == "" {
		log.Fatalf("HOSTBUILDER_URL is required.")
	}
	hostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	dbURL := os.Getenv("NITROUS_PG_URL")
	if dbURL == "" {
		log.Fatal("NITROUS_PG_URL is required.")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database, err: %v", err)
	}

	var (
		numWorkers = 5 // At most 5 workers will run at any one time.
		wg         sync.WaitGroup
		jobs       = make(chan *job, 100)
	)

	// Start workers.
	for i := 0; i < numWorkers; i++ {
		go worker(dbConn, &wg, jobs)
	}

	users := []*user.User{}
	// TODO We should select smaller batches by varying LIMIT and OFFSET so that
	// the query runs faster and doesn't consume a lot of memory when populating
	// the slice.
	if err := dbConn.Select(&users, `
		SELECT DISTINCT(users.*) FROM users
		JOIN hosts ON users.id = hosts.user_id
		WHERE
			users.deleted_at IS NULL
			AND users.service_account = false
			AND hosts.terminated_at IS NULL
		ORDER BY ID;
	`); err != nil {
		return err
	}

	for _, u := range users {
		hosts, err := host.FindAllByUser(dbConn, u.ID)
		if err != nil {
			return err
		}

		log.Printf("User ID %d (%q) has %d hosts", u.ID, u.Username, len(hosts))

		for _, h := range hosts {
			wg.Add(1)
			jobs <- &job{user: u, host: h}
		}
	}

	wg.Wait()

	return nil
}

func worker(db *db.Database, wg *sync.WaitGroup, jobs chan *job) {
	for jb := range jobs {
		err := archive(db, jb.user, jb.host)
		if err != nil {
			log.Errorf("error archiving host ID %d, err: %v", jb.host.ID, err)
		}

		wg.Done()
	}
}

func archive(db *db.Database, u *user.User, h *host.Host) error {
	os.MkdirAll("./logs", 0775)

	f, err := os.Create(fmt.Sprintf("logs/host-%d.log", h.ID))
	if err != nil {
		return err
	}
	defer f.Close()

	mw := io.MultiWriter(f, os.Stdout)

	hostLog := log.New()
	hostLog.Out = mw

	hostLog.Printf("Host ID %d: in the %q state", h.ID, h.State)

	switch h.State {
	case host.StateStopped:
		hostLog.Printf("Host ID %d: starting...", h.ID)
		info, err := hostbuilderClient.Start(
			h.Provider, h.MachineConfig(), nil, nil)
		if err != nil {
			return err
		}

		h.State = host.StateRunning
		h.ProviderIdentifier = sql.NullString{info.ID, true}
		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
		if err := db.WithTransaction(func(tx *sqlx.Tx) error {
			columnsToUpdate := []string{"state", "provider_identifier", "ip_address", "freighterd_port"}
			if err := h.UpdateSpecifiedColumns(tx, columnsToUpdate); err != nil {
				return err
			}
			return hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt, 0)
		}); err != nil {
			return err
		}

		hostLog.Printf("Host ID %d: successfully started: %#v", h.ID, info)
	case host.StateRunning:
		// Fall through (i.e. continue).
	default:
		// TODO We should log to a machine-readable logfile so that we
		// can do re-processing, inform user of errors, etc.
		hostLog.Printf("Host ID %d: in the %q state, skipping...", h.ID, h.State)
		return errors.New("host is in an unprocessable state")
	}

	// TODO Should we check state NOT IN ('created_failed', 'creating').
	containers, err := container.FindAllByHostID(db, h.ID)
	if err != nil {
		return err
	}

	hk, err := hostkeys.FromSlug(db, h.Slug)
	if err != nil {
		return err
	}

	privKeyPEM, err := hk.PrivatePEM()
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(privKeyPEM)
	if err != nil {
		return err
	}

	hostLog.Printf("Host ID %d: going to SSH into host...", h.ID)
	client, err := ssh.Dial("tcp", hk.Address(), &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	})
	if err != nil {
		return err
	}
	hostLog.Printf("Host ID %d: SSH success.", h.ID)

	hostLog.Printf("Host ID %d: uploading shell script...", h.ID)
	if err := uploadShellScript(client); err != nil {
		return err
	}
	hostLog.Printf("Host ID %d: shell script uploaded.", h.ID)

	for _, cn := range containers {
		start := time.Now()

		// TODO We can also use S3 pre-signed URLs but those expire in 7 days.
		containerHash := genContainerHash(cn.Slug, u.EncryptedPassword)

		env := []string{
			"AWS_ACCESS_KEY_ID=" + awsAccessKey,
			"AWS_SECRET_ACCESS_KEY=" + awsSecretKey,
			"AWS_REGION=us-east-1",
			"BUCKET=" + s3Bucket,
			"CONTAINER=" + cn.Slug,
			"CONTAINER_HASH=" + cn.Slug + "-" + containerHash,
		}
		envStr := strings.Join(env, " ")
		cmd := "sudo " + envStr + " ./backup-container.sh"

		hostLog.Printf("Host ID %d: backing up container %q...", h.ID, cn.Slug)

		session, err := client.NewSession()
		if err != nil {
			return err
		}
		defer session.Close()

		session.Stdout = mw
		session.Stderr = mw

		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("error running script, err: %v", err)
		}

		hostLog.Printf("Host ID %d: backed up container %q to s3://%s/%s in %v", h.ID, cn.Slug, s3Bucket, containerHash, time.Since(start))
	}

	return nil
}

func uploadShellScript(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	f, err := os.Open("./backup-container.sh")
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		fmt.Fprintf(w, "C0764 %d %s\n", stat.Size(), "backup-container.sh")
		io.Copy(w, f)
		fmt.Fprint(w, "\x00")
	}()

	cmd := fmt.Sprintf("scp -t %s", "/home/ubuntu/backup-container.sh")
	if out, err := session.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("error scp-ing shell script to host, err: %v, output: %s", err, out)
	}

	return nil
}

func genContainerHash(container, password string) string {
	h := sha256.New()
	h.Write([]byte(container))
	h.Write([]byte("-"))
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil))
}
