package cops

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-fsnotify/fsnotify"
)

var (
	copsFeatures = map[string]interface{}{
		// for every optional feature cops provides, add entry mapping to true:
		// "thisFeatureIsAvailable" : true,
		"hasCmdRun": true,
	}
)

type OpsHandler struct {
	containerId     string
	mu              sync.Mutex
	errCh           chan error
	decoder         *json.Decoder
	encoder         *json.Encoder
	fileWatcher     *fsnotify.Watcher
	fileWatcherQuit chan bool
	watchedDirs     map[string]bool

	uid, gid int
}

func handleOps(info *CopsInfo, r io.Reader, w io.Writer) (err error) {
	uid, gid := info.owner()
	oh := &OpsHandler{
		containerId:     info.CID,
		decoder:         json.NewDecoder(r),
		encoder:         json.NewEncoder(w),
		errCh:           make(chan error, 1),
		fileWatcherQuit: make(chan bool),
		watchedDirs:     make(map[string]bool),
		uid:             uid,
		gid:             gid,
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

// messageLoop runs inside a chroot environment and processes container requests
func (oh *OpsHandler) copsMessageLoop() {
	// cache handlers
	fsHandler := NewFsHandler(oh.uid, oh.gid)
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
