package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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

	csvWriter *csv.Writer

	okFile *os.File
	okMu   sync.Mutex

	failedFile *os.File
	failedMu   sync.Mutex
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

	ts := time.Now().Unix()

	// Record successful/failed host backups to files.
	okFile, err = os.Create(fmt.Sprintf("successful-hosts-%d.txt", ts))
	if err != nil {
		return err
	}
	defer okFile.Close()

	failedFile, err = os.Create(fmt.Sprintf("failed-hosts-%d.csv", ts))
	if err != nil {
		return err
	}
	defer failedFile.Close()

	// Log to a global CSV file. We'll use this to send emails to users with
	// links to their container archives.
	csvFile := fmt.Sprintf("archiver-%d.csv", ts)
	log.Infof("Logging results to CSV file at %q", csvFile)
	f, err := os.Create(csvFile)
	if err != nil {
		return err
	}
	defer f.Close()

	csvWriter = csv.NewWriter(f)
	csvWriter.Write([]string{
		"email", "username", "container slug", "container name",
		"archive URL", "archive created at",
	})
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return err
	}

	var (
		numWorkers = 10
		wg         sync.WaitGroup
		jobs       = make(chan *job, 100)
	)

	if d, err := strconv.ParseInt(os.Getenv("NUM_WORKERS"), 10, 64); err == nil {
		numWorkers = int(d)
	}

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
		ORDER BY id;
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

			failedMu.Lock()
			if _, err := failedFile.WriteString(fmt.Sprintf("%d, %v\n", jb.host.ID, err)); err != nil {
				log.Errorf("error writing to host ID %d failure to record file: %v", jb.host.ID, err)
			}
			failedFile.Sync()
			failedMu.Unlock()
		} else {
			okMu.Lock()
			if _, err := okFile.WriteString(fmt.Sprintf("%d\n", jb.host.ID)); err != nil {
				log.Errorf("error writing to host ID %d success to record file: %v", jb.host.ID, err)
			}
			okFile.Sync()
			okMu.Unlock()
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

	var stopAfter bool
	defer func() {
		if stopAfter {
			hostLog.Printf("Host ID %d: returning to stopped state...", h.ID)
			_, err := hostbuilderClient.Stop(
				h.Provider, h.MachineConfig(), nil, nil)
			if err != nil {
				log.Errorf("Host ID %d: failed to stop host, err: %v", h.ID, err)
				return
			}

			h.State = host.StateStopped
			h.ProviderIdentifier = sql.NullString{}
			h.IPAddress = sql.NullString{}
			h.FreighterdPort = sql.NullInt64{}
			if err := db.WithTransaction(func(tx *sqlx.Tx) error {
				columnsToUpdate := []string{"state", "provider_identifier", "ip_address", "freighterd_port"}
				if err := h.UpdateSpecifiedColumns(tx, columnsToUpdate); err != nil {
					return err
				}
				return hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt, 0)
			}); err != nil {
				log.Errorf("Host ID %d: failed update after stopping, err: %v", h.ID, err)
				return
			}
		}
	}()

	switch h.State {
	case host.StateStopped:
		hostLog.Printf("Host ID %d: starting...", h.ID)
		info, err := hostbuilderClient.Start(
			h.Provider, h.MachineConfig(), nil, nil)
		if err != nil {
			return err
		}

		h.State = host.StateRunning
		h.ProviderIdentifier = sql.NullString{String: info.ID, Valid: true}
		h.IPAddress = sql.NullString{String: info.IPAddress, Valid: true}
		h.FreighterdPort = sql.NullInt64{Int64: int64(info.FreighterdPort), Valid: true}
		if err := db.WithTransaction(func(tx *sqlx.Tx) error {
			columnsToUpdate := []string{"state", "provider_identifier", "ip_address", "freighterd_port"}
			if err := h.UpdateSpecifiedColumns(tx, columnsToUpdate); err != nil {
				return err
			}
			return hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt, 0)
		}); err != nil {
			return err
		}

		stopAfter = true

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
		log.Errorf("error getting private key in PEM format: %v", err)
		return err
	}

	signer, err := ssh.ParsePrivateKey(privKeyPEM)
	if err != nil {
		log.Errorf("error getting parsing private key: %v", err)
		return err
	}

	hostLog.Printf("Host ID %d: going to SSH into host...", h.ID)
	client, err := ssh.Dial("tcp", hk.Address(), &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
	})
	if err != nil {
		log.Errorf("error connecting to host at %v: %v", hk.Address(), err)
		return err
	}
	hostLog.Printf("Host ID %d: SSH success.", h.ID)

	hostLog.Printf("Host ID %d: uploading container archiver binary...", h.ID)
	binPath, err := uploadBinary(client)
	if err != nil {
		log.Errorf("error uploading container archiver binary: %v", err)
		return err
	}
	defer func() {
		session, err := client.NewSession()
		if err != nil {
			log.Errorf("error starting new SSH session: %v", err)
		}
		defer session.Close()

		if err := session.Run(fmt.Sprintf(`rm "%s"`, binPath)); err != nil {
			log.Errorf("error deleting container archiver binary: %v", err)
		}
	}()
	hostLog.Printf("Host ID %d: container archiver binary uploaded.", h.ID)

	for _, cn := range containers {
		start := time.Now()

		// TODO We can also use S3 pre-signed URLs but those expire in 7 days.
		containerHash := genContainerHash(cn.Slug)
		containerFilename := fmt.Sprintf("user-%d-%s/%s-%s",
			u.ID, u.Username, cn.Slug, containerHash)

		env := []string{
			"AWS_ACCESS_KEY_ID=" + awsAccessKey,
			"AWS_SECRET_ACCESS_KEY=" + awsSecretKey,
			"AWS_REGION=us-east-1",
			"BUCKET=" + s3Bucket,
			"CONTAINER=" + cn.Slug,
			"CONTAINER_HASH=" + containerFilename,
		}
		envStr := strings.Join(env, " ")
		cmd := "sudo " + envStr + " " + binPath

		hostLog.Printf("Host ID %d: backing up container %q...", h.ID, cn.Slug)

		session, err := client.NewSession()
		if err != nil {
			log.Errorf("error starting new SSH session: %v", err)
			return err
		}
		defer session.Close()

		session.Stdout = mw
		session.Stderr = mw

		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("error running script, err: %v", err)
		}

		hostLog.Printf("Host ID %d: backed up container %q to s3://%s/%s in %v", h.ID, cn.Slug, s3Bucket, containerFilename, time.Since(start))

		uri := fmt.Sprintf("https://s3.amazonaws.com/%s/%s.tar.gz",
			s3Bucket, containerFilename)

		// email, username, container slug, container name, URL, timestamp of backup
		csvWriter.Write([]string{
			u.Email, u.Username, cn.Slug, cn.Name.String,
			uri, strconv.FormatInt(start.Unix(), 10),
		})
		csvWriter.Flush()

		if err := csvWriter.Error(); err != nil {
			log.Errorf("error writing to CSV: %v", err)
			return err
		}
	}

	return nil
}

func uploadBinary(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	f, err := os.Open("./containerarchiver")
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()

		fmt.Fprintf(w, "C0764 %d %s\n", stat.Size(), "containerarchiver")
		io.Copy(w, f)
		fmt.Fprint(w, "\x00")
	}()

	path := fmt.Sprintf("/home/ubuntu/containerarchiver-%d", time.Now().Unix())

	cmd := fmt.Sprintf(`scp -t "%s"`, path)
	if out, err := session.CombinedOutput(cmd); err != nil {
		return "", fmt.Errorf("error scp-ing binary to host, err: %v, output: %s", err, out)
	}

	return path, nil
}

func genContainerHash(container string) string {
	// Generate a randomly long enough string. We don't care about repeatability
	// because we'll save the generated hash.

	h := sha256.New()
	h.Write([]byte(container))
	h.Write([]byte("-"))

	rand, _ := alphanumeric(256)
	h.Write([]byte(rand))

	hash := hex.EncodeToString(h.Sum(nil))

	rand, _ = alphanumeric(100)

	return hash + rand
}

const alphanumericDict = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func alphanumeric(n uint) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	bl := byte(len(alphanumericDict))
	for k, v := range b {
		b[k] = alphanumericDict[v%bl]
	}
	return string(b), nil
}
