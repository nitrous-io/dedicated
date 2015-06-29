package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

var (
	compassPrefix     = "compass | "
	tenderPrefix      = "tender  | "
	nitrousPrefix     = "nitrous | "
	hostbuilderPrefix = "hostbdr | "

	m      sync.Mutex
	stdout *os.File
)

func main() {
	// check if we have dnsmasq to setup the internal DNS nameserver
	if err := checkDNS(); err != nil {
		logrus.Fatal(err)
	}
	if err := copyCert(); err != nil {
		logrus.Fatal(err)
	}
	if err := pullImage("ubuntu"); err != nil {
		logrus.Fatal(err)
	}

	stdout = os.Stdout

	id := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		panic(err)
	}
	uuid := hex.EncodeToString(id)

	var (
		compassContainer = fmt.Sprintf("compass-%s", uuid)
		tenderContainer  = fmt.Sprintf("tender-%s", uuid)
		compass          = compassCommand(compassContainer, stdout, compassPrefix)
		tender           = tenderCommand(tenderContainer, stdout, tenderPrefix)
		nitrous          = nitrousCommand(stdout, nitrousPrefix)
		hostbuilder      = hostbuilderCommand(stdout, hostbuilderPrefix)
	)

	closeFn := func() {
		logrus.Println("....closing")
		if compass.Process != nil {
			compass.Process.Signal(os.Interrupt)
		}

		if tender.Process != nil {
			tender.Process.Signal(os.Interrupt)
		}

		if nitrous.Process != nil {
			nitrous.Process.Signal(os.Interrupt)
		}
		if hostbuilder.Process != nil {
			hostbuilder.Process.Signal(os.Interrupt)
		}
		removeDNS()
		logrus.Println("closed....")
	}

	defer closeFn()

	var (
		compassErr     = promise(compass)
		tenderErr      = promise(tender)
		nitrousErr     = promise(nitrous)
		hostbuilderErr = promise(hostbuilder)
	)

	// wait for everything to start
	time.Sleep(1 * time.Second)
	compassIP, err := getIP(compassContainer)
	if err != nil {
		logrus.Error(err)
		return
	}

	// write and restart dnsmasq settings
	writeDNS([]dnsServer{
		dnsServer{name: "n2odevpro.com", ip: compassIP},
		dnsServer{name: "n2odevproxy.com", ip: compassIP},
		dnsServer{name: "n2odevapp.com", ip: compassIP},
	})

	// Set up channel on which to send signal notifications.
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	logrus.Println(".....READY!!")
	select {
	case err := <-compassErr:
		logrus.Errorf("compass: FAILED %v", err)
	case err := <-tenderErr:
		logrus.Errorf("tender: FAILED %v", err)
	case err := <-nitrousErr:
		logrus.Errorf("nitrous: FAILED %v", err)
	case err := <-hostbuilderErr:
		logrus.Errorf("hostbuilder: FAILED %v", err)
	case <-c:
		logrus.Print("intercept close signal")
	}
	return
}

func pullImage(image string) error {
	logrus.Printf("Pulling %s...", image)
	err := exec.Command("docker", "run", "--rm", image, "true").Run()
	if err != nil {
		return err
	}
	logrus.Printf("Pulled %s", image)
	return nil
}

func getIP(containerName string) (string, error) {
	output, err := exec.Command("docker", "inspect", "-f",
		"{{.NetworkSettings.IPAddress}}", containerName).Output()
	if err != nil {
		return "", fmt.Errorf("failed to find IP of %q", containerName)
	}
	return string(output), nil
}

func checkDNS() error {
	_, err := exec.LookPath("dnsmasq")
	if err != nil {
		return fmt.Errorf("dnsmasq not found. Please install it: apt-get install dnsmasq")
	}
	return nil
}

func copyCert() error {
	_, err := os.Stat("/usr/local/share/ca-certificates/n2odev.crt")
	if os.IsNotExist(err) {
		err = exec.Command("sudo", "cp", "./script/certs/n2odev.crt",
			"/usr/local/share/ca-certificates/n2odev.crt").Run()
		if err != nil {
			return err
		}
		logrus.Println("reloading certificates...")
		return exec.Command("sudo", "update-ca-certificates").Run()
	}
	logrus.Println("n2odev certificate is installed. Skipping...")
	return nil
}

type dnsServer struct {
	name string
	ip   string
}

// write dns file and reset
func writeDNS(servers []dnsServer) error {
	confName := "/etc/dnsmasq.d/99compass"
	cmd := exec.Command("sudo", "tee", confName)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to open tee's stdin for %s", confName)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to tee to %s, err %q", confName, err)
	}
	for _, s := range servers {
		fmt.Fprintln(stdin, fmt.Sprintf("server=/%s/%s", s.name, s.ip))
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to tee to %s, err %q", confName, err)
	}

	out, err := exec.Command("sudo", "service", "dnsmasq", "restart").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart dnsmasq %s", string(out))
	}
	logrus.Println("dnsmasq restarted....")
	return nil
}

func removeDNS() error {
	confName := "/etc/dnsmasq.d/99compass"
	exec.Command("sudo", "rm", confName).Run()
	out, err := exec.Command("sudo", "service", "dnsmasq", "restart").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart dnsmasq %s", string(out))
	}
	logrus.Println("dnsmasq restarted....")
	return nil
}

func compassCommand(name string, stdout io.Writer, prefix string) *exec.Cmd {
	wd, _ := os.Getwd()
	w := newPrefixWriter(stdout, prefix)
	cmd := exec.Command("docker", "run",
		"-v", fmt.Sprintf("%s:/compass", filepath.Join(wd, "build", "compass")),
		"-v", fmt.Sprintf("%s:/config.json", filepath.Join(wd, "compass", "config.json")),
		"-v", "/run/postgresql:/run/postgresql",
		"-p", "53",
		"--rm",
		"--name", name,
		"-i", "ubuntu",
		"/compass", "-c", "/config.json",
	)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

func tenderCommand(name string, stdout io.Writer, prefix string) *exec.Cmd {
	wd, _ := os.Getwd()
	w := newPrefixWriter(stdout, prefix)
	cmd := exec.Command("docker", "run",
		"-v", fmt.Sprintf("%s:/tender", filepath.Join(wd, "build", "tender")),
		"-v", fmt.Sprintf("%s:/certs", filepath.Join(wd, "script", "certs")),
		"-v", "/run/postgresql:/run/postgresql",
		"--env-file", filepath.Join(wd, "tender", ".env"),
		"--rm",
		"--net", "host", // run net=host so it can interact with other containers
		"--name", name,
		"-i", "ubuntu",
		"/tender",
	)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

func nitrousCommand(stdout io.Writer, prefix string) *exec.Cmd {
	w := newPrefixWriter(stdout, prefix)
	cmd := exec.Command("forego", "start", "-f", "nitrous/Procfile.dev", "-e", "nitrous/.env")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

func hostbuilderCommand(stdout io.Writer, prefix string) *exec.Cmd {
	w := newPrefixWriter(stdout, prefix)
	cmd := exec.Command("./build/hostbuilder", "-config", "./hostbuilder/config.json")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd
}

func promise(cmd *exec.Cmd) chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := cmd.Start(); err != nil {
			errCh <- err
			return
		}
		errCh <- cmd.Wait()
	}()
	return errCh
}

type prefixWriter struct {
	pr     *io.PipeReader
	pw     *io.PipeWriter
	writer io.Writer
	prefix string
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	return w.pw.Write(p)
}

func (w *prefixWriter) drain() {
	scanner := bufio.NewScanner(w.pr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m.Lock()
		fmt.Fprintln(w.writer, w.prefix, line)
		m.Unlock()
	}
	w.pr.CloseWithError(scanner.Err())
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	pr, pw := io.Pipe()
	writer := &prefixWriter{pr: pr, pw: pw, writer: w, prefix: prefix}
	go writer.drain()
	return writer
}
