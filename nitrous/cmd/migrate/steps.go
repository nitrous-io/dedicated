package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/crypto/ssh"
)

type namedConn struct {
	conn *sshutil.Conn
	name string
}

type namedDB struct {
	host *host.Host
	name string
}

func allSSHHosts() []namedConn {
	return []namedConn{{srcSSHHost, "SRC"}, {destSSHHost, "DEST"}}
}

func allDBHosts() []namedDB {
	return []namedDB{{srcDBHost, "SRC"}, {destDBHost, "DEST"}}
}

func homePathForUser(user string) string {
	if user == "root" {
		return "/root"
	}
	return fmt.Sprintf("/home/%s", user)
}

func verifyCommunication() error {
	for _, hst := range allSSHHosts() {
		out, err := hst.conn.CombinedOutput("uname -a")
		if err != nil {
			return handleFatalError(err, hst)
		}
		handleOutput(out, hst)
	}
	return nil
}

// setSSHKeys() will generate a SSH keypair, send the private key to the destination
// and the public key to the source. This allows the destination to rsync directly with
// the source machine
func setSSHKeys() error {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("Error calling GenerateKey: %v", err)
	}

	data, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return fmt.Errorf("Error calling NewPublicKey: %v", err)
	}
	pubkey := ssh.MarshalAuthorizedKey(data)

	privder := x509.MarshalPKCS1PrivateKey(priv)
	privblk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privder,
	}
	prikey := pem.EncodeToMemory(&privblk)

	pubRd := bytes.NewReader(pubkey)
	privRd := bytes.NewReader(prikey)

	dHome := homePathForUser(destSSHHost.Config.User)
	sHome := homePathForUser(srcSSHHost.Config.User)

	logr.Info("[DEST] Uploading private key")
	destSSHHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", dHome))
	loc := fmt.Sprintf("%s/.ssh/id_rsa", dHome)
	if err := destSSHHost.Upload(loc, privRd); err != nil {
		return fmt.Errorf("DEST] Error uploading private key: %v", err)
	}
	destSSHHost.CombinedOutput(fmt.Sprintf("chmod 0600 %s", loc))

	logr.Info("[SRC] Uploading temporary public key for appending")
	srcSSHHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", sHome))
	loc = fmt.Sprintf("%s/.ssh/authorized_keys.tmp", sHome)
	if err := srcSSHHost.Upload(loc, pubRd); err != nil {
		return fmt.Errorf("[SRC] Error uploading temporary public key: %v", err)
	}

	logr.Info("[SRC] Appending temporary public key")
	srcSSHHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", sHome))
	out, err := srcSSHHost.CombinedOutput(fmt.Sprintf("cat %s >> '%s/.ssh/authorized_keys'", loc, sHome))
	if err != nil {
		return fmt.Errorf("[SRC] Error appending temporary public key: %s err: %v", out, err)
	}

	logr.Info("[DEST] Testing SSH comms with SRC")
	parts := strings.Split(srcSSHHost.Client.RemoteAddr().String(), ":")
	cmd := fmt.Sprintf(`ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null %s@%s -p %s 'echo "welcome!"'`, srcSSHHost.Config.User, parts[0], parts[1])
	out, err = destSSHHost.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("[DEST] Error testing comms with SRC: %s err: %v", out, err)
	}
	if !strings.Contains(string(out), "welcome!") {
		return fmt.Errorf("[DEST] Error looking for `welcome!`: %s", out)
	}
	logr.Info("[DEST] Test succeeded!")

	return nil
}

func markServers() error {
	tx, err := dbConn.Beginx()
	if err != nil {
		return fmt.Errorf("Unable to begin transaction for record updates. err: %v", err)
	}

	for _, nm := range allDBHosts() {
		nm.host.State = "migrating"
		_, err = tx.Exec("UPDATE hosts SET state = $1 WHERE id = $2", nm.host.State, nm.host.ID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("[%s] Unable to set migrating state. err: %v", nm.name, err)
		}

		err = hosttransition.Record(tx, nm.host, "running", time.Now())
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("[%s] Unable to transition host. err: %v", nm.name, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Unable to commit transaction for record updates. err: %v", err)
	}

	return nil
}

func restoreServers() error {
	tx, err := dbConn.Beginx()
	if err != nil {
		return fmt.Errorf("Unable to begin transaction for record updates. err: %v", err)
	}

	logr.Info("[SRC] Setting state to running")
	// leave DST state in migrating so users don't see it
	nm := namedDB{srcDBHost, "SRC"}
	nm.host.State = "running"
	_, err = tx.Exec("UPDATE hosts SET state = $1 WHERE id = $2", nm.host.State, nm.host.ID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[%s] Unable to update host state. err: %v", nm.name, err)
	}

	err = hosttransition.Record(tx, nm.host, "migrating", time.Now())
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[%s] Unable to transition host. err: %v", nm.name, err)
	}

	logr.Info("[SRC] Starting docker")
	out, err := srcSSHHost.CombinedOutput("sudo service docker start")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[SRC] Could not start Docker. %s err: %v", out, err)
	}

	logr.Info("[SRC] Starting freighterd")
	out, err = srcSSHHost.CombinedOutput("sudo service freighterd start")
	if err != nil {
		return fmt.Errorf("[SRC] Could not start Freighter. %s err: %v", out, err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Unable to commit transaction for record updates. err: %v", err)
	}

	return nil
}

func stopServices() error {
	logr.Info("[ALL] Stopping freighter & docker services on DEST & SRC...")
	for _, hst := range allSSHHosts() {
		for _, c := range []string{"freighterd", "docker"} {
			cmd := fmt.Sprintf("sudo service %s status", c)
			out, err := hst.conn.CombinedOutput(cmd)
			if strings.Contains(string(out), "stop/waiting") || strings.Contains(string(out), "is not running") {
				continue
			}

			handleOutput(out, hst)
			if err != nil {
				return handleFatalError(err, hst)
			}

			cmd = fmt.Sprintf("sudo service %s stop", c)
			out, err = hst.conn.CombinedOutput(cmd)
			handleOutput(out, hst)
			if err != nil {
				return handleFatalError(err, hst)
			}
		}
	}

	return nil
}

func getTransferrableDirectories() []string {
	dirs := []string{"/var/lib/docker", "/var/lib/freighter"}

	chk, _ := srcSSHHost.CombinedOutput("ls -la /.dockercfg")
	if !strings.Contains(string(chk), "No such file or directory") {
		dirs = append(dirs, "/.dockercfg")
	} else {
		logr.Warn("[DEST] /.dockercfg will not be transferred from SRC, does not exit")
	}

	return dirs
}

func backupDestDirectories(dirs []string) error {
	for _, dir := range dirs {
		logr.Infof("[DEST] Backing up %s", dir)
		cmd := fmt.Sprintf("sudo mv %s{,.bak}", dir)
		out, err := destSSHHost.CombinedOutput(cmd)
		if err != nil {
			return fmt.Errorf("[DEST] Failed to backup `%s`: %s err: %v", dir, out, err)
		}

		chk, _ := destSSHHost.CombinedOutput(fmt.Sprintf("ls -la %s.bak", dir))
		if strings.Contains(string(chk), "No such file or directory") {
			return fmt.Errorf("[DEST] %s.bak could not be located", dir)
		}
	}

	return nil
}

func removeBackupDestDirectories(dirs []string) error {
	for _, dir := range dirs {
		bdir := fmt.Sprintf("%s.bak", dir)
		logr.Infof("[DEST] Removing %s", bdir)
		cmd := fmt.Sprintf("sudo rm -r %s", bdir)
		out, err := destSSHHost.CombinedOutput(cmd)
		if err != nil {
			return fmt.Errorf("[DEST] Failed to remove `%s`: %s err: %v", dir, out, err)
		}
	}
	return nil
}

func syncDirectories(dirs []string) error {
	dHome := homePathForUser(destSSHHost.Config.User)
	parts := strings.Split(srcSSHHost.Client.RemoteAddr().String(), ":")
	remoteCmd := fmt.Sprintf(`ssh -i '%s/.ssh/id_rsa' -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -p %s`, dHome, parts[1])

	// run rsync with --ignore-existing --sparse first to handle sparse files,
	// then run rsync with --inplace to update files in place.
	// See http://serverfault.com/questions/66338/how-do-you-synchronise-huge-sparse-files-vm-disk-images-between-machines/435514#435514
	for _, dir := range dirs {
		cmd := fmt.Sprintf(`sudo rsync --ignore-existing --sparse -az --rsync-path="sudo rsync" -e "%s" %s@%s:%s/ %s`, remoteCmd, srcSSHHost.Config.User, parts[0], dir, dir)
		logr.Debugf("[DEST] Running %s", cmd)
		out, err := destSSHHost.CombinedOutput(cmd)
		if err != nil {
			return fmt.Errorf("[DEST] Error rsyncing `%s`: %s err: %v", dir, out, err)
		}

		cmd = fmt.Sprintf(`sudo rsync --inplace -az --rsync-path="sudo rsync" -e "%s" %s@%s:%s/ %s`, remoteCmd, srcSSHHost.Config.User, parts[0], dir, dir)
		logr.Debugf("[DEST] Running %s", cmd)
		out, err = destSSHHost.CombinedOutput(cmd)
		if err != nil {
			return fmt.Errorf("[DEST] Error rsyncing `%s`: %s err: %v", dir, out, err)
		}

		chk, _ := destSSHHost.CombinedOutput(fmt.Sprintf("ls -la %s", dir))
		if strings.Contains(string(chk), "No such file or directory") {
			return fmt.Errorf("[DEST] %s could not be located after rsync", dir)
		}
		logr.Infof("[DEST] Successfully rsynced %s", dir)
	}

	return nil
}

func sanityCheck() error {
	for _, hst := range allSSHHosts() {
		out, err := hst.conn.CombinedOutput("sudo service docker start && sleep 2")
		if err != nil {
			handleOutput(out, hst)
			return handleFatalError(err, hst)
		}
	}

	defer func() {
		for _, hst := range allSSHHosts() {
			out, err := hst.conn.CombinedOutput("sudo service docker stop && sleep 2")
			if err != nil {
				handleOutput(out, hst)
				handleFatalError(err, hst)
			}
		}
	}()

	// output of docker ps -a -s looks like
	// CONTAINER ID        IMAGE                                       COMMAND                CREATED             STATUS                      PORTS                                            NAMES               SIZE
	// 82a32f8fda1e        quay.io/nitrous/golang:latest               "/sbin/init --defaul   5 days ago          Exited (0) 18 minutes ago   0.0.0.0:49167->22/tcp, 0.0.0.0:49168->6060/tcp   terra-4791          254 MB

	// after cmd pipeline, above should look like:
	// 82a32f8fda1e quay.io/nitrous/golang:latest terra-4791 254MB

	cmd := `sudo docker ps -a -s | tail -n +2 | sort -n | awk '{ print $1, $2, $(NF-2), $(NF-1)$NF }'`

	dstPs, err := destSSHHost.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("[DEST] Could not obtain docker ps -a. %s err: %v", dstPs, err)
	}

	srcPs, err := srcSSHHost.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("[SRC] Could not obtain docker ps -a. %s err: %v", srcPs, err)
	}

	if string(srcPs) != string(dstPs) {
		// sometimes there can be small discrepancies in size, so prompt user
		// to check
		fmt.Printf("[ALL] docker ps -a -s does not match:\n[SRC]\n%s\n[DEST]\n%s\n",
			srcPs, dstPs)
		var in string
		for {
			fmt.Printf("Please check the destination host then type T to continue with the migration: ")
			if _, err := fmt.Scanln(&in); err != nil {
				fmt.Printf("Error in input: %s\n", err)
			} else {
				break
			}
		}
		resume, _ := strconv.ParseBool(in)
		if !resume {
			return fmt.Errorf("[ALL] docker ps -a -s does not match\n[SRC] %s\n[DEST] %s", srcPs, dstPs)
		}
	}

	return nil
}

func updateRecords() error {
	tx, err := dbConn.Beginx()
	if err != nil {
		return fmt.Errorf("Unable to begin transaction for record updates. err: %v", err)
	}

	_, err = tx.Exec("DELETE FROM volumes WHERE host_id = $1 AND slug = 'shared';", destDBHost.ID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[DEST] Unable to remove existing shared volumes. err: %v", err)
	}

	for _, tbl := range []string{"containers", "volumes", "images"} {
		logr.Infof("[DEST] Updating table %s to host_id=%d from host_id=%d", tbl, destDBHost.ID, srcDBHost.ID)
		qry := fmt.Sprintf("UPDATE %s SET host_id = $1 WHERE host_id = $2;", tbl)
		_, err := tx.Exec(qry, destDBHost.ID, srcDBHost.ID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("Unable to update %s table. err: %v", tbl, err)
		}
	}

	destDBHost.State = "running"
	_, err = tx.Exec("UPDATE hosts SET state = $1 WHERE id = $2;", destDBHost.State, destDBHost.ID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[DEST] Unable to set state. err: %v", err)
	}

	err = hosttransition.Record(tx, destDBHost, "migrating", time.Now())
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[DEST] Unable to transition host. err: %v", err)
	}

	srcDBHost.Slug = fmt.Sprintf("migrated-%s", srcHostSlug)
	var oldName string
	if srcDBHost.Name.Valid {
		oldName = srcDBHost.Name.String
		srcDBHost.Name = sql.NullString{fmt.Sprintf("migrated-%s", srcDBHost.Name.String), true}
	}
	if err := srcDBHost.Update(tx); err != nil {
		tx.Rollback()
		return fmt.Errorf("[SRC] Unable to update slug/name '%s'. err: %v", srcDBHost.Slug, err)
	}

	_, err = tx.Exec("UPDATE hosts SET state = 'terminated', terminated_at = now() WHERE id = $1", srcDBHost.ID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[SRC] Unable to set terminated_at state. err: %v", err)
	}

	srcDBHost.State = "terminated"
	err = hosttransition.Record(tx, srcDBHost, "migrating", time.Now())
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("[SRC] Unable to transition host. err: %v", err)
	}

	destDBHost.Slug = srcHostSlug
	destDBHost.Secret = srcDBHost.Secret
	if oldName != "" {
		destDBHost.Name = sql.NullString{oldName, true}
	}
	if err := destDBHost.Update(tx); err != nil {
		tx.Rollback()
		return fmt.Errorf("[DEST] Unable to update slug to %s. err: %v", destDBHost.Slug, err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Unable to commit transaction for record updates. err: %v", err)
	}
	return nil
}

func startServices() error {
	logr.Info("[DEST] Starting docker")
	out, err := destSSHHost.CombinedOutput("sudo service docker start && sleep 2")
	if err != nil {
		return fmt.Errorf("[DEST] Could not start Docker. %s err: %v", out, err)
	}

	chk, _ := destSSHHost.CombinedOutput("sudo docker ps -a")
	logr.Infof("[DEST] %s", chk)

	logr.Info("[DEST] Starting freighterd")
	out, err = destSSHHost.CombinedOutput("sudo service freighterd start")
	if err != nil {
		return fmt.Errorf("[DEST] Could not start Freighter. %s err: %v", out, err)
	}
	return nil
}

func stopDestination() error {
	bearer := fmt.Sprintf("Authorization: Bearer %s", uToken.Token)
	url := fmt.Sprintf("https://pro.nitrous.io/api/v0/hosts/%s/stop", destDBHost.Slug)
	cmd := exec.Command("curl", "-X", "POST", "-H", bearer, url)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	cmd.Wait()

	fmt.Println()

	var count int
	err := dbConn.Get(&count, `SELECT COUNT(*) FROM hosts WHERE id = $1 AND state = 'stopped';`, destDBHost.ID)
	if err != nil {
		return fmt.Errorf("Error fetching final host, err: %v", err)
	}

	if count != 1 {
		return fmt.Errorf("Failed to verify %d was stopped", destDBHost.ID)
	}

	return nil
}

func handleFatalError(err error, w namedConn) error {
	return fmt.Errorf("[%s] Err: %v", w.name, err)
}

func handleOutput(out []byte, w namedConn) {
	logr.Debugf("[%s] %s", w.name, out)
}
