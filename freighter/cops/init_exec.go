package cops

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/airbrake"
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
	id := flag.Arg(0)

	if err := syscall.Chdir("/"); err != nil {
		airbrake.Fatalf("execInit failed to change dir to / : %v", err)
	}

	// this is used to communicate with the parent process
	pipe := os.NewFile(3, "pipe")
	defer pipe.Close()
	err := handleOps(id, os.Stdin, pipe)

	if err != nil {
		airbrake.Fatalf("opsHandler failed with %v", err)
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
	airbrake.Printf("NITROUS_USERNAME not set")
	return "nitrous"
}

// gets a shell for the user as decoded from /etc/passwd or /bin/bash if
// failed to decode /etc/passwd
func shellForUserFromEtcPasswd(user string) string {
	d, err := ioutil.ReadFile("/etc/passwd")
	if err != nil {
		airbrake.Printf("ioutil.ReadFile('/etc/passwd') failed with %s", err)
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
		airbrake.Printf("su shell '%s' cmd: '%s' failed with %s, stdout: '%s', stderr: '%s'\n", shell, cmdStr, err, stdout, stderr)
	} else {
		airbrake.Printf("su shell '%s' cmd: '%s' returned stdout: '%s' stderr: '%s'\n", shell, cmdStr, stdout, stderr)
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
		airbrake.Printf("msg.Args[0] (%#v) is not a list of strings", msg.Args[0])
		sendErr(ch, msg, errors.New("invalid cmd argument"))
		return
	}
	if len(cmdWithArgs) == 0 {
		airbrake.Printf("msg.Args[0] is an empty list")
		sendErr(ch, msg, errors.New("empty cmd argument"))
		return
	}
	cmdStr := shellquote.CmdToString(cmdWithArgs)

	// this probably doesn't matter as sudo tries to change the dir
	// set custom working directory (if given)
	cwd, ok := msg.Args[1].(string)
	if !ok {
		airbrake.Printf("msg.Args[1] (%#v) is not a string", msg.Args[1])
		sendErr(ch, msg, errors.New("invalid cwd argument"))
		return
	}
	cmd := buildWithSuUserShell(cmdStr)
	cmd.Dir = strings.TrimSpace(cwd)

	// setup additional environment variables (if given)
	env, ok := getArgAsStrArray(msg.Args[2])
	if !ok {
		airbrake.Printf("msg.Args[2] (%#v) is not a list of strings", msg.Args[2])
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
			airbrake.Printf("cmd: '%s' args: '%#v', cmdStr: %s failed with '%s'", cmdName, args, cmdStr, err)
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
		airbrake.Printf("CmdHandler.HandleAsync(): unknown event '%s'", msg.Event)
		return sendErr(resp, msg, fmt.Errorf("unkown event '%s'", msg.Event))
	}
	return resp
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
