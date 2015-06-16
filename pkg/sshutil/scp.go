package sshutil

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/Sirupsen/logrus"
)

func (c *Conn) scpSession(scpCommand string, f func(io.Writer, *bufio.Reader) error) error {
	session, err := c.newSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Get a pipe to stdin so that we can send data down
	stdinW, err := session.StdinPipe()
	if err != nil {
		return err
	}
	// Get a pipe to stdout so that we can get responses back
	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stdoutR := bufio.NewReader(stdoutPipe)

	// We only want to close once, so we nil w after we close it,
	// and only close in the defer if it hasn't been closed already.
	defer func() {
		if stdinW != nil {
			stdinW.Close()
		}
	}()

	// Set stderr to a bytes buffer
	stderr := new(bytes.Buffer)
	session.Stderr = stderr

	// Start the sink mode on the other side
	logrus.Debug("Starting remote scp process: ", scpCommand)
	if err := session.Start(scpCommand); err != nil {
		return err
	}

	// Call our callback that executes in the context of SCP. We ignore
	// EOF errors if they occur because it usually means that SCP prematurely
	// ended on the other side.
	logrus.Debug("Started SCP session, beginning transfers...")
	if err := f(stdinW, stdoutR); err != nil && err != io.EOF {
		return err
	}

	// Close the stdin, which sends an EOF, and then set w to nil so that
	// our defer func doesn't close it again since that is unsafe with
	// the Go SSH package.
	logrus.Debug("SCP session complete, closing stdin pipe.")
	stdinW.Close()
	stdinW = nil

	// Wait for the SCP connection to close, meaning it has consumed all
	// our data and has completed. Or has errored.
	return session.Wait()
}

func scpUploadFile(dst string, src io.Reader, w io.Writer, r *bufio.Reader) error {
	tf, err := ioutil.TempFile("", "ssh-upload")
	if err != nil {
		return fmt.Errorf("Failed to create temp file: %s", err)
	}
	defer func() {
		tf.Close()
		os.Remove(tf.Name())
	}()
	if _, err := io.Copy(tf, src); err != nil {
		return fmt.Errorf("Failed to copy data to temp file: %s", err)
	}
	// Sync the file so that the contents are definitely on disk, then
	// read the length of it.
	if err := tf.Sync(); err != nil {
		return fmt.Errorf("Failed to sync temp file for upload: %s", err)
	}
	// Seek the file to the beginning so we can re-read all of it
	if _, err := tf.Seek(0, 0); err != nil {
		return fmt.Errorf("Failed to seek to beginning of file: %s", err)
	}
	// Get stat of the file to calculate size
	fi, err := tf.Stat()
	if err != nil {
		return fmt.Errorf("Failed to get file stat: %s", err)
	}
	// Start the protocol
	fmt.Fprintln(w, "C0644", fi.Size(), dst)
	if err := checkSCPStatus(r); err != nil {
		return err
	}
	if _, err := io.Copy(w, tf); err != nil {
		return err
	}
	fmt.Fprint(w, "\x00")
	return checkSCPStatus(r)
}

// checkSCPStatus checks that a prior command sent to SCP completed
// successfully. If it did not complete successfully, an error will
// be returned.
func checkSCPStatus(r *bufio.Reader) error {
	code, err := r.ReadByte()
	if err != nil {
		return err
	}
	if code != 0 {
		// Treat any non-zero (really 1 and 2) as fatal errors
		message, _, err := r.ReadLine()
		if err != nil {
			return fmt.Errorf("Error reading error message: %s", err)
		}
		return errors.New(string(message))
	}
	return nil
}
