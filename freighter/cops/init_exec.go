package cops

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/go-fsnotify/fsnotify"
	"github.com/nitrous-io/dedicated/pkg/reexec"
	"github.com/nitrous-io/dedicated/pkg/shellquote"
)

const (
	execCommandName = "cops-exec"
	defaultShell    = "/bin/bash"
)

var (
	copsFeatures = map[string]interface{}{
		// for every optional feature cops provides, add entry mapping to true:
		// "thisFeatureIsAvailable" : true,
		"hasCmdRun": true,
	}
	mu          sync.Mutex
	userToShell map[string]string
)

func init() {
	reexec.Register(execCommandName, execInit)
	userToShell = make(map[string]string)
}

// execInit runs inside the container mnt namespace, and processes websocket messages
func execInit() {
	runtime.LockOSThread()
	flag.Parse()
	id := flag.Arg(0)

	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("execInit failed to change dir to / : %v", err)
	}

	// this is used to communicate with the parent process
	pipe := os.NewFile(3, "pipe")
	defer pipe.Close()
	err := handleOps(id, os.Stdin, pipe)

	if err != nil {
		log.Fatalf("opsHandler failed with %v", err)
	}
	os.Exit(0)
}

type OpsHandler struct {
	containerId     string
	mu              sync.Mutex
	errCh           chan error
	decoder         *json.Decoder
	encoder         *json.Encoder
	fileWatcher     *fsnotify.Watcher
	fileWatcherQuit chan bool
	watchedDirs     map[string]bool
}

func handleOps(containerId string, r io.Reader, w io.Writer) error {
	var err error
	oh := &OpsHandler{
		containerId:     containerId,
		decoder:         json.NewDecoder(r),
		encoder:         json.NewEncoder(w),
		errCh:           make(chan error, 1),
		fileWatcherQuit: make(chan bool),
		watchedDirs:     make(map[string]bool),
	}
	oh.fileWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go oh.fileWatcherLoop()
	go oh.copsMessageLoop()
	return oh.waitFinished()
}

func (oh *OpsHandler) stopWithError(err error) {
	oh.fileWatcherQuit <- true
	oh.errCh <- err
}

func (oh *OpsHandler) waitFinished() error {
	err := <-oh.errCh
	return err
}

func (oh *OpsHandler) sendReply(msg *Message) error {
	oh.mu.Lock()
	err := oh.encoder.Encode(msg)
	oh.mu.Unlock()
	if err != nil {
		log.Printf("sendReply: oh.encoder.Encode() failed with '%s'", err)
		oh.stopWithError(fmt.Errorf("failed to encode %v, %s", msg, err))
	}
	return err
}

func (oh *OpsHandler) notifyDirChanged(dir string) {
	msg := &Message{
		Namespace: "watcher",
		Event:     "dirChanged.output",
	}
	msg.Args = []interface{}{dir}
	oh.sendReply(msg)
}

func (oh *OpsHandler) sendFeatures(orig *Message) {
	msg := &Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Args:      []interface{}{copsFeatures},
	}
	oh.sendReply(msg)
}

func (oh *OpsHandler) replyWithError(orig *Message, errMsg string) {
	msg := &Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Error:     errMsg,
	}
	oh.sendReply(msg)
}

func shouldNotifyAboutEvent(e fsnotify.Event) bool {
	if e.Op&fsnotify.Create == fsnotify.Create {
		return true
	}
	if e.Op&fsnotify.Remove == fsnotify.Remove {
		return true
	}
	if e.Op&fsnotify.Rename == fsnotify.Rename {
		return true
	}
	return false
}

// changes to some files shouldn't trigger updates.
// we should definitely filter out ".zsh_history.LOCK" (and possibly similar
// files for other shells), as they are updated after every command in console
// this is a heuristic
func shouldIgnoreFile(path string) bool {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	return false
}

func (oh *OpsHandler) fileWatcherLoop() {
	defer oh.fileWatcher.Close()
	for {
		select {
		case event := <-oh.fileWatcher.Events:
			path := event.Name
			if shouldNotifyAboutEvent(event) && !shouldIgnoreFile(path) {
				// it's the parent directory that needs to be updated
				oh.notifyDirChanged(filepath.Dir(path))
			}
		case err := <-oh.fileWatcher.Errors:
			log.Printf("fileWatcherLoop: got fsnotify error '%s'\n", err)
			return
		case <-oh.fileWatcherQuit:
			log.Printf("quitting file watcher\n")
			return
		}
	}
}

func (oh *OpsHandler) runHandler(h AsyncHandler, msg *Message) {
	ch := h.HandleAsync(msg)
	for reply := range ch {
		if err := oh.sendReply(reply); err != nil {
			return
		}
	}
}

type CmdHandler struct {
}

func getNitrousUser() string {
	user := os.Getenv("NITROUS_USERNAME")
	if user != "" {
		return user
	}
	// shouldn't happen, but if it does, log the error and go on using "nitrous" user
	log.Printf("getNitrousUser: NITROUS_USERNAME not set")
	return "nitrous"
}

// gets a shell for the user as decoded from /etc/passwd or /bin/bash if
// failed to decode /etc/passwd
func shellForUserFromEtcPasswd(user string) string {
	d, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		log.Printf("shellForUserFromEtcPasswd: ioutil.ReadFile('/etc/passwd') failed with %s", err)
		return defaultShell
	}
	shell := extractShellFromEtcPasswd(string(d), user)
	if shell != "" {
		return shell
	}
	return defaultShell
}

func getShellForUserCached(user string) string {
	mu.Lock()
	defer mu.Unlock()
	if shell, ok := userToShell[user]; ok {
		return shell
	}
	shell := shellForUserFromEtcPasswd(user)
	userToShell[user] = shell
	return shell
}

// we're running as root and need to execute user's commands under their
// user privileges/shell environment
// su [options] ${username}
// options:
//  -p : preserve the env
//  -c ${cmd} : execute a command
// shell options:
//  -l : login
//  -c ${cmd} : command to execute
func buildWithSuUserShell(cmdStr string) *exec.Cmd {
	shell := getShellForUserCached(getNitrousUser())
	cmdStr = shell + " -l -c " + shellquote.QuoteCmd(cmdStr)
	return exec.Command("/bin/su", "-p", "-c", cmdStr, getNitrousUser())
}

func runCmd(cmd *exec.Cmd) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	stdoutStr := string(stdout.Bytes())
	stderrStr := removeBogusFromStderr(string(stderr.Bytes()))
	return stdoutStr, stderrStr, err
}

func runWithSuUserShell(cmdStr string) (string, string, error) {
	cmd := buildWithSuUserShell(cmdStr)
	stdout, stderr, err := runCmd(cmd)
	shell := getShellForUserCached(getNitrousUser())
	if err != nil {
		log.Printf("su shell '%s' cmd: '%s' failed with %s, stdout: '%s', stderr: '%s'\n", shell, cmdStr, err, stdout, stderr)
	} else {
		log.Printf("su shell '%s' cmd: '%s' returned stdout: '%s' stderr: '%s'\n", shell, cmdStr, stdout, stderr)
	}
	return stdout, stderr, err
}

// arguments in msg.Args:
// - 1st is a list of strings that is a command to execute with arguments e.g. ["ls", "-lah"]
// - 2nd is current workding directory or empty string to use what we have
// - 3rd is a list of additional environment strings e.g. ["MYPATH=foo", "YOURPATH=bar"].
//   can be an empty list
func doRun(msg *Message, ch chan *Message) {
	if len(msg.Args) != 3 {
		sendErr(ch, msg, errors.New("invalid number of arguments, expects 3"))
		return
	}
	cmdWithArgs, ok := getArgAsStrArray(msg.Args[0])
	if !ok {
		log.Printf("doRun: msg.Args[0] (%#v) is not a list of strings", msg.Args[0])
		sendErr(ch, msg, errors.New("invalid cmd argument"))
		return
	}
	if len(cmdWithArgs) == 0 {
		log.Printf("doRun: msg.Args[0] is an empty list")
		sendErr(ch, msg, errors.New("empty cmd argument"))
		return
	}
	cmdStr := shellquote.CmdToString(cmdWithArgs)

	// this probably doesn't matter as sudo tries to change the dir
	// set custom working directory (if given)
	cwd, ok := msg.Args[1].(string)
	if !ok {
		log.Printf("doRun: msg.Args[1] (%#v) is not a string", msg.Args[1])
		sendErr(ch, msg, errors.New("invalid cwd argument"))
		return
	}
	cmd := buildWithSuUserShell(cmdStr)
	cmd.Dir = strings.TrimSpace(cwd)

	// setup additional environment variables (if given)
	env, ok := getArgAsStrArray(msg.Args[2])
	if !ok {
		log.Printf("doRun: msg.Args[2] (%#v) is not a list of strings", msg.Args[2])
		sendErr(ch, msg, errors.New("invalid env argument"))
		return
	}
	// our env is additive and cmd.Env replaces, so we create a superset ourselves
	if len(env) > 0 {
		for _, e := range env {
			cmd.Env = append(cmd.Env, e)
		}
	}

	go func(cmd *exec.Cmd) {
		errMsg := ""
		stdout, stderr, err := runCmd(cmd)
		if err != nil {
			errMsg = err.Error()
			cmdName := cmdWithArgs[0]
			args := cmdWithArgs[1:]
			log.Printf("doRun: cmd: '%s' args: '%#v', cmdStr: %s failed with '%s'", cmdName, args, cmdStr, err)
		}
		args := []interface{}{errMsg, stdout, stderr}
		sendFinalReply(ch, msg, args)
	}(cmd)
}

func (h *CmdHandler) HandleAsync(msg *Message) chan *Message {
	resp := make(chan *Message, chanSize)
	switch msg.Event {
	case "run":
		go doRun(msg, resp)
	default:
		log.Printf("CmdHandler.HandleAsync(): unknown event '%s'", msg.Event)
		return sendErr(resp, msg, fmt.Errorf("unkown event '%s'", msg.Event))
	}
	return resp
}

// messageLoop runs inside a chroot environment and processes container requests
func (oh *OpsHandler) copsMessageLoop() {
	// cache handlers
	fsHandler := NewFsHandler()
	finderHandler := NewFinderHandler()
	cmdHandler := &CmdHandler{}

	for {
		msg := &Message{}
		err := oh.decoder.Decode(msg)
		if err != nil {
			oh.stopWithError(fmt.Errorf("failed to decode msg %s", err))
			return
		}

		// handle operations in a separate goroutine to not block the
		// message pipeline
		switch msg.Namespace {
		case "fs":
			switch msg.Event {
			case "list":
				dir := msg.Args[0].(string)
				// avoid watching same dir twice - unclear if fsnotify handles that
				// for all platforms
				if !oh.watchedDirs[dir] {
					oh.watchedDirs[dir] = true
					oh.fileWatcher.Add(dir)
				}
			case "unwatchDir":
				dir := msg.Args[0].(string)
				delete(oh.watchedDirs, dir)
				oh.fileWatcher.Remove(dir)
			}
			go oh.runHandler(fsHandler, msg)
		case "features":
			switch msg.Event {
			case "get":
				oh.sendFeatures(msg)
			default:
				log.Printf("copsMessageLoop() unknown event '%s' in namespace 'features'", msg.Event)
				oh.replyWithError(msg, fmt.Sprintf("invalid operation, namespace: '%s', event: '%s'", msg.Namespace, msg.Event))
			}
		case "finder":
			go oh.runHandler(finderHandler, msg)
		case "cmd":
			go oh.runHandler(cmdHandler, msg)
		default:
			// on unknown messages we send an error back. this allows IDE to work
			// with older versions of freighter (as long as it sensibly handles this
			// message)
			log.Printf("copsMessageLoop() invalid operation, namespace: '%s', event: '%s'", msg.Namespace, msg.Event)
			oh.replyWithError(msg, fmt.Sprintf("invalid operation, namespace: '%s', event: '%s'", msg.Namespace, msg.Event))
		}
	}
}

// Start listening to messages meant for container with `id`, spins up a
// chroot-d process for that container and proxy data in/out of the chrooted
// process.
// This is run in the parent process
func Start(id string, env []string) (*exec.Cmd, io.Writer, io.Reader, error) {
	initPID, err := GetContainerPID(id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get container %s pid: %v", id, err)
	}

	// we use pipe for output of cops process because its stdout/stderr may
	// contains logs generated by child process
	copsStdout, child, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}

	// parent doesn't need the child
	defer child.Close()

	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(execCommandName, strconv.Itoa(initPID))
	if len(env) != 0 {
		for _, e := range os.Environ() {
			env = append(env, e)
		}
		cmd.Env = env
	}
	copsStdin, err := cmd.StdinPipe()
	if err != nil {
		copsStdout.Close()
		return nil, nil, nil, err
	}
	cmd.ExtraFiles = []*os.File{child}
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		copsStdout.Close()
		return nil, nil, nil, err
	}
	return cmd, copsStdin, copsStdout, nil
}
