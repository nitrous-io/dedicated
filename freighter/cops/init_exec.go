package cops

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/go-fsnotify/fsnotify"
	"github.com/nitrous-io/dedicated/pkg/reexec"
)

const execCommandName = "cops-exec"

var (
	enableDebugLogs = true
)

func init() {
	reexec.Register(execCommandName, execInit)
}

// execInit runs inside the container mnt namespace, and process websocket messages
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
	if enableDebugLogs {
		s, _ := json.MarshalIndent(msg, "", "  ")
		log.Printf("sendReply: %s\n", s)
	}
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
	log.Printf("sendFeatures()\n")
	features := map[string]interface{}{
		"saveFileWithCmd": true,
	}
	msg := &Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Args:      []interface{}{features},
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

// messageLoop runs inside a chroot environment and processes container requests
func (oh *OpsHandler) copsMessageLoop() {
	// cache handlers
	fsHandler := NewFsHandler()
	finderHandler := NewFinderHandler()

	for {
		msg := &Message{}
		err := oh.decoder.Decode(msg)
		if err != nil {
			oh.stopWithError(fmt.Errorf("failed to decode msg %s", err))
			return
		}

		if enableDebugLogs {
			s, _ := json.MarshalIndent(msg, "", "  ")
			log.Printf("copsMessageLoop() got: %s", s)
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
		case "features2":
			switch msg.Event {
			case "get":
				oh.sendFeatures(msg)
			default:
				// TODO: this should be error log
				log.Printf("copsMessageLoop() unknown event '%s' in namespace 'features'", msg.Event)
			}
		case "finder":
			go oh.runHandler(finderHandler, msg)
		default:
			// on unknown messages we send an error back. this allows IDE to work
			// with older versions of freighter (as long as it sensibly handles this
			// message)
			oh.replyWithError(msg, fmt.Sprintf("invalid operation, namespace: '%s', event: '%s'", msg.Namespace, msg.Event))
			return
		}
	}
}

// Start listening to messages meant for container with `id`, spins up a
// chroot-d process for that container and proxy data in/out of the chrooted
// process.
// This is run in the parent process
func Start(id string, r io.Reader) (done chan error, quit chan error, msg chan map[string]interface{}, err error) {
	initPID, err := GetContainerPID(id)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get container %s pid: %v", id, err)
	}

	parent, child, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}

	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(execCommandName, strconv.Itoa(initPID))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	cmd.ExtraFiles = []*os.File{child}
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// parent doesnt need the child, so close it asap
	if err := cmd.Start(); err != nil {
		child.Close()
		return nil, nil, nil, err
	}
	child.Close()

	msgs := make(chan map[string]interface{})
	// copy data in and out of the child process
	// input goes to stdin
	// output comes from the pipe, not stdout because stdout/stderr may
	// contains logs generated by the child process
	quit = make(chan error, 2)
	go func() {
		// read json messages from cops process and send them to the caller
		dec := json.NewDecoder(parent)
		var err error
		for {
			var msg map[string]interface{}
			if err = dec.Decode(&msg); err == nil {
				msgs <- msg
			} else {
				break
			}
		}
		quit <- err
	}()
	go func() {
		_, err := io.Copy(stdin, r)
		quit <- err
	}()
	// listen to quit and kill the commmand whenever the pipes are closed
	go func() {
		<-quit
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()
	done = make(chan error)
	// write to error channel whenever the command finishes
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	return done, quit, msgs, nil
}
