package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/crypto/ssh"
)

type namedConn struct {
	conn *sshutil.Conn
	name string
}

func allHosts() []namedConn {
	return []namedConn{{srcHost, "SRC"}, {destHost, "DEST"}}
}

func homePathForUser(user string) string {
	if user == "root" {
		return "/root"
	}
	return fmt.Sprintf("/home/%s", user)
}

func verifyCommunication() {
	for _, hst := range allHosts() {
		out, err := hst.conn.CombinedOutput("uname -a")
		if err != nil {
			handleFatalError(err, hst)
		}
		handleOutput(out, hst)
	}
}

// setSSHKeys() will generate a SSH keypair, send the private key to the destination
// and the public key to the source. This allows the destination to rsync directly with
// the source machine
func setSSHKeys() {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		logr.FatalfExit("Error calling GenerateKey: %v", err)
	}

	data, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		logr.FatalfExit("Error calling NewPublicKey: %v", err)
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

	dHome := homePathForUser(destHost.Config.User)
	sHome := homePathForUser(srcHost.Config.User)

	logr.Info("[DEST] Uploading private key")
	destHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", dHome))
	loc := fmt.Sprintf("%s/.ssh/id_rsa", dHome)
	if err := destHost.Upload(loc, privRd); err != nil {
		logr.FatalfExit("DEST] Error uploading private key: %v", err)
	}
	destHost.CombinedOutput(fmt.Sprintf("chmod 0600 %s", loc))

	logr.Info("[SRC] Uploading temporary public key for appending")
	srcHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", sHome))
	loc = fmt.Sprintf("%s/.ssh/authorized_keys.tmp", sHome)
	if err := srcHost.Upload(loc, pubRd); err != nil {
		logr.FatalfExit("[SRC] Error uploading temporary public key: %v", err)
	}

	logr.Info("[SRC] Appending temporary public key")
	srcHost.CombinedOutput(fmt.Sprintf("mkdir -p '%s/.ssh'", sHome))
	out, err := srcHost.CombinedOutput(fmt.Sprintf("cat %s >> '%s/.ssh/authorized_keys'", loc, sHome))
	if err != nil {
		logr.FatalfExit("[SRC] Error appending temporary public key: %s err: %v", out, err)
	}

	logr.Info("[DEST] Testing SSH comms with SRC")
	parts := strings.Split(srcHost.Client.RemoteAddr().String(), ":")
	cmd := fmt.Sprintf(`ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null %s@%s -p %s 'echo "welcome!"'`, srcHost.Config.User, parts[0], parts[1])
	out, err = destHost.CombinedOutput(cmd)
	if err != nil {
		logr.FatalfExit("[DEST] Error testing comms with SRC: %s err: %v", out, err)
	}
	if !strings.Contains(string(out), "welcome!") {
		logr.FatalfExit("[DEST] Error looking for `welcome!`: %s", out)
	}
	logr.Info("[DEST] Test succeeded!")
}

func stopServices() {
	logr.Info("[ALL] Stopping freighter & docker services on DEST & SRC...")
	for _, hst := range allHosts() {
		for _, c := range []string{"freighterd", "docker"} {
			cmd := fmt.Sprintf("sudo service %s status", c)
			out, err := hst.conn.CombinedOutput(cmd)
			if strings.Contains(string(out), "stop/waiting") || strings.Contains(string(out), "is not running") {
				continue
			}

			handleOutput(out, hst)
			if err != nil {
				handleFatalError(err, hst)
			}

			cmd = fmt.Sprintf("sudo service %s stop", c)
			out, err = hst.conn.CombinedOutput(cmd)
			handleOutput(out, hst)
			if err != nil {
				handleFatalError(err, hst)
			}
		}
	}
}

func getTransferrableDirectories() []string {
	dirs := []string{"/var/lib/docker", "/var/lib/freighter"}

	chk, _ := srcHost.CombinedOutput("ls -la /.dockercfg")
	if !strings.Contains(string(chk), "No such file or directory") {
		dirs = append(dirs, "/.dockercfg")
	} else {
		logr.Warn("[DEST] /.dockercfg will not be transferred from SRC, does not exit")
	}

	return dirs
}

func backupDestDirectories(dirs []string) {
	for _, dir := range dirs {
		logr.Infof("[DEST] Backing up %s", dir)
		cmd := fmt.Sprintf("sudo mv %s{,.bak}", dir)
		out, err := destHost.CombinedOutput(cmd)
		if err != nil {
			logr.FatalfExit("[DEST] Failed to backup `%s`: %s err: %v", dir, out, err)
		}

		chk, _ := destHost.CombinedOutput(fmt.Sprintf("ls -la %s.bak", dir))
		if strings.Contains(string(chk), "No such file or directory") {
			logr.FatalfExit("[DEST] %s.bak could not be located", dir)
		}
	}
}

func syncDirectories(dirs []string) {
	dHome := homePathForUser(destHost.Config.User)

	for _, dir := range dirs {
		parts := strings.Split(srcHost.Client.RemoteAddr().String(), ":")
		cmd := fmt.Sprintf(`sudo rsync -az --rsync-path="sudo rsync" -e "ssh -i '%s/.ssh/id_rsa' -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -p %s" %s@%s:%s/ %s`, dHome, parts[1], srcHost.Config.User, parts[0], dir, dir)
		logr.Debugf("[DEST] Running %s", cmd)
		out, err := destHost.CombinedOutput(cmd)
		if err != nil {
			logr.FatalfExit("[DEST] Error rsyncing `%s`: %s err: %v", dir, out, err)
		}

		chk, _ := destHost.CombinedOutput(fmt.Sprintf("ls -la %s", dir))
		if strings.Contains(string(chk), "No such file or directory") {
			logr.FatalfExit("[DEST] %s could not be located after rsync", dir)
		}
		logr.Infof("[DEST] Successfully rsynced %s", dir)
	}
}

func sanityCheck() bool {
	for _, hst := range allHosts() {
		out, err := hst.conn.CombinedOutput("sudo service docker start && sleep 2")
		if err != nil {
			handleOutput(out, hst)
			handleFatalError(err, hst)
		}
	}

	defer func() {
		for _, hst := range allHosts() {
			out, err := hst.conn.CombinedOutput("sudo service docker stop && sleep 2")
			if err != nil {
				handleOutput(out, hst)
				handleFatalError(err, hst)
			}
		}
	}()

	dstPs, err := destHost.CombinedOutput(`sudo docker ps -a -s | sed "s/[[:space:]][[:space:]]\+/|/g" | cut -d"|" -f1,2,3,6,7,8`)
	if err != nil {
		logr.Fatalf("[DEST] Could not obtain docker ps -a. %s err: %v", dstPs, err)
		return false
	}

	srcPs, err := srcHost.CombinedOutput(`sudo docker ps -a -s | sed "s/[[:space:]][[:space:]]\+/|/g" | cut -d"|" -f1,2,3,6,7,8`)
	if err != nil {
		logr.Fatalf("[SRC] Could not obtain docker ps -a. %s err: %v", srcPs, err)
		return false
	}

	if string(srcPs) != string(dstPs) {
		logr.Fatalf("[ALL] docker ps -a -s does not match")
		logr.Fatalf("[SRC] %s", srcPs)
		logr.Fatalf("[DEST] %s", dstPs)
		return false
	}

	return true
}

func updateRecords() {
	tx, err := dbConn.Beginx()
	if err != nil {
		logr.FatalfExit("Unable to begin transaction for record updates. err: %v", err)
	}
	dHost, err := host.FindByIdentifier(dbConn, destHostSlug)
	if err != nil {
		tx.Rollback()
		logr.FatalfExit("[DEST] Unable to locate host record. err: %v", err)
	}

	sHost, err := host.FindByIdentifier(dbConn, srcHostSlug)
	if err != nil {
		tx.Rollback()
		logr.FatalfExit("[SRC] Unable to locate host record. err: %v", err)
	}

	_, err = tx.Exec("DELETE FROM volumes WHERE host_id = $1 AND slug = 'shared';", dHost.ID)
	if err != nil {
		tx.Rollback()
		logr.FatalfExit("[DEST] Unable to remove existing shared volumes. err: %v", err)
	}

	for _, tbl := range []string{"containers", "volumes", "images"} {
		logr.Infof("[DEST] Updating table %s from host_id=%d to host_id=%d", tbl, dHost.ID, sHost.ID)
		qry := fmt.Sprintf("UPDATE %s SET host_id = $1 WHERE host_id = $2;", tbl)
		_, err := tx.Exec(qry, dHost.ID, sHost.ID)
		if err != nil {
			tx.Rollback()
			logr.FatalfExit("Unable to update %s table. err: %v", tbl, err)
		}
	}

	sHost.Slug = fmt.Sprintf("migrated-%s", srcHostSlug)
	var oldName string
	if sHost.Name.Valid {
		oldName = sHost.Name.String
		sHost.Name = sql.NullString{fmt.Sprintf("migrated-%s", sHost.Name.String), true}
	}
	if err := sHost.Update(tx); err != nil {
		tx.Rollback()
		logr.FatalfExit("[SRC] Unable to update slug/name. err: %v", sHost.Slug, err)
	}

	_, err = tx.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1", sHost.ID)
	if err != nil {
		tx.Rollback()
		logr.FatalfExit("[SRC] Unable to set terminated_at state. err: %v", err)
	}

	dHost.Slug = srcHostSlug
	dHost.Secret = sHost.Secret
	if oldName != "" {
		dHost.Name = sql.NullString{oldName, true}
	}
	if err := dHost.Update(tx); err != nil {
		tx.Rollback()
		logr.FatalfExit("[DEST] Unable to update slug to %s. err: %v", dHost.Slug, err)
	}

	err = tx.Commit()
	if err != nil {
		logr.FatalfExit("Unable to commit transaction for record updates. err: %v", err)
	}
}

func startServices() {
	logr.Info("[DEST] Starting docker")
	out, err := destHost.CombinedOutput("sudo service docker start && sleep 2")
	if err != nil {
		logr.FatalfExit("[DEST] Could not start Docker. %s err: %v", out, err)
	}

	chk, _ := destHost.CombinedOutput("sudo docker ps -a")
	logr.Infof("[DEST] %s", chk)

	logr.Info("[DEST] Starting freighterd")
	out, err = destHost.CombinedOutput("sudo service freighterd start")
	if err != nil {
		logr.FatalfExit("[DEST] Could not start Freighter. %s err: %v", out, err)
	}
}

func handleFatalError(err error, w namedConn) {
	logr.FatalfExit("[%s] Err: %v", w.name, err)
}

func handleOutput(out []byte, w namedConn) {
	logr.Debugf("[%s] %s", w.name, out)
}
