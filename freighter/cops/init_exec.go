package cops

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/pkg/reexec"
	"github.com/nitrous-io/dedicated/pkg/shellquote"
)

const (
	execCommandName = "cops-exec"
	defaultShell    = "/bin/bash"
)

var (
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
	// required init pid to be argv 0th

	if err := syscall.Chdir("/"); err != nil {
		logrus.Fatalf("execInit failed to change dir to / : %v", err)
	}

	// this is used to get info of the container from the parent
	infoR := os.NewFile(3, "info")
	defer infoR.Close()
	info := &CopsInfo{}
	if err := json.NewDecoder(infoR).Decode(info); err != nil {
		logrus.Fatalf("%s cmd: failed to decode info %v", execCommandName, err)
	}

	// this is used to communicate with the parent process
	pipe := os.NewFile(4, "pipe")
	defer pipe.Close()
	if err := handleOps(info, os.Stdin, pipe); err != nil {
		logrus.Fatalf("opsHandler failed with %v", err)
	}
	os.Exit(0)
}

type CmdHandler struct {
}

func getNitrousUser() string {
	user := os.Getenv("NITROUS_USERNAME")
	if user != "" {
		return user
	}
	// shouldn't happen, but if it does, log the error and go on using "nitrous" user
	logrus.Error("getNitrousUser: NITROUS_USERNAME not set")
	return "nitrous"
}

// gets a shell for the user as decoded from /etc/passwd or /bin/bash if
// failed to decode /etc/passwd
func shellForUserFromEtcPasswd(user string) string {
	d, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		logrus.Errorf("shellForUserFromEtcPasswd: ioutil.ReadFile('/etc/passwd') failed with %s", err)
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

func buildWithLoginUserShell(cmdStr string) *exec.Cmd {
	nitrousEnv := []string{}
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "NITROUS_") {
			nitrousEnv = append(nitrousEnv, env)
		}
	}
	if len(nitrousEnv) > 0 {
		cmdStr = fmt.Sprintf("%s %s", strings.Join(nitrousEnv, " "), cmdStr)
	}
	shell := getShellForUserCached(getNitrousUser())
	cmdStr = shell + " -i -l -c " + shellquote.QuoteCmd(cmdStr)
	return exec.Command("/bin/su", "--login", "-c", cmdStr, getNitrousUser())
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
		logrus.Errorf("doRun: msg.Args[0] (%#v) is not a list of strings", msg.Args[0])
		sendErr(ch, msg, errors.New("invalid cmd argument"))
		return
	}
	if len(cmdWithArgs) == 0 {
		logrus.Error("doRun: msg.Args[0] is an empty list")
		sendErr(ch, msg, errors.New("empty cmd argument"))
		return
	}
	cmdStr := shellquote.CmdToString(cmdWithArgs)

	// this probably doesn't matter as sudo tries to change the dir
	// set custom working directory (if given)
	cwd, ok := msg.Args[1].(string)
	if !ok {
		logrus.Errorf("doRun: msg.Args[1] (%#v) is not a string", msg.Args[1])
		sendErr(ch, msg, errors.New("invalid cwd argument"))
		return
	}
	var cmd *exec.Cmd
	if msg.Event == "loginrun" {
		cmd = buildWithLoginUserShell(cmdStr)
	} else {
		cmd = buildWithSuUserShell(cmdStr)
	}
	cmd.Dir = strings.TrimSpace(cwd)

	// setup additional environment variables (if given)
	env, ok := getArgAsStrArray(msg.Args[2])
	if !ok {
		logrus.Errorf("doRun: msg.Args[2] (%#v) is not a list of strings", msg.Args[2])
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
			logrus.Errorf("doRun: cmd: '%s' args: '%#v', cmdStr: %s failed with '%s'", cmdName, args, cmdStr, err)
		}
		args := []interface{}{errMsg, stdout, stderr}
		sendFinalReply(ch, msg, args)
	}(cmd)
}

func (h *CmdHandler) HandleAsync(msg *Message) chan *Message {
	resp := make(chan *Message, chanSize)
	switch msg.Event {
	case "run", "loginrun":
		go doRun(msg, resp)
	default:
		logrus.Errorf("CmdHandler.HandleAsync(): unknown event '%s'", msg.Event)
		return sendErr(resp, msg, fmt.Errorf("unkown event '%s'", msg.Event))
	}
	return resp
}

// Start listening to messages meant for container with `id`, spins up a
// chroot-d process for that container and proxy data in/out of the chrooted
// process.
// This is run in the parent process
func Start(id string, env []string) (*exec.Cmd, io.Writer, io.Reader, error) {
	info, err := getCopsInfo(id, env)
	if err != nil {
		return nil, nil, nil, err
	}
	// create a pipe to pass info over
	infoR, infoW, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}
	defer infoR.Close()

	// we use pipe for output of cops process because its stdout/stderr may
	// contains logs generated by child process
	copsStdout, child, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}
	// parent doesn't need the child
	defer child.Close()

	// create the command, pass the child as fd3 of the command
	cmd := newCommand(execCommandName, info.PID)
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
	cmd.ExtraFiles = []*os.File{infoR, child}
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		copsStdout.Close()
		return nil, nil, nil, err
	}
	if err := json.NewEncoder(infoW).Encode(info); err != nil {
		logrus.Errorf("%s cmd: failed to write info %v", execCommandName, err)
		cmd.Process.Kill()
		return nil, nil, nil, err
	}
	return cmd, copsStdin, copsStdout, nil
}
