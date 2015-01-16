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
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/reexec"
)

type (
	// Message will be serialized over the transport. It is used to represent both
	// the operation and its result.
	Message struct {
		Namespace string        `json:"namespace"`
		Event     string        `json:"event"`
		CID       int64         `json:"cid"`
		Args      []interface{} `json:"args"`
		Error     string        `json:"error"`
	}

	// Handler is the actual operation that operates on the container `id`.
	// TODO: deprecate this interface and upgrade implementation to AsyncHandler
	// e.g. cops/fs
	Handler interface {
		Handle(msg *Message) *Message
	}

	FileInfo struct {
		CID   string
		Name  string
		IsDir bool
	}

	// this will be used when handler supports this interface
	AsyncHandler interface {
		HandleAsync(msg *Message) chan *Message
	}

	NewHandler func(id string) (Handler, error)
)

var (
	Handlers   = map[string]NewHandler{}
	RootPrefix = "/var/lib/docker/aufs/mnt"

	// list of commands
	execCommandName     = "cops-exec"
	copyCommandName     = "cops-copy"
	infoCommandName     = "cops-fileinfo"
	readFileCommandName = "cops-readfile"
)

func init() {
	reexec.Register(execCommandName, copsInit)
	reexec.Register(infoCommandName, infoInit)
	reexec.Register(readFileCommandName, readFileInit)
}

// copyInit chroot itself inside the container diretory, and process cops
// websocket messages
func copsInit() {
	runtime.LockOSThread()
	flag.Parse()
	id := flag.Arg(0)
	path := flag.Arg(1)

	// chroot the current process into path
	if err := syscall.Chroot(path); err != nil {
		log.Fatalf("copsInit failed to chroot to %s : %v", path, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("copsInit failed to change dir to / : %v", err)
	}

	// this is used to communicate with the parent process
	pipe := os.NewFile(3, "pipe")
	defer pipe.Close()
	errCh := ListenAndServe(id, os.Stdin, pipe)
	if err := <-errCh; err != nil {
		log.Fatalf("failed to serve cops websocket %v", err)
	}
	os.Exit(0)
}

// infoInit chroot itself inside the container diretory, and return fileinfo of
// a specific path
func infoInit() {
	runtime.LockOSThread()
	flag.Parse()
	id := flag.Arg(0)
	path := flag.Arg(1)
	localPath := flag.Arg(2)

	// chroot the current process into path
	if err := syscall.Chroot(path); err != nil {
		log.Fatalf("infoInit failed to chroot to %s : %v", path, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("infoInit failed to change dir to / : %v", err)
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		log.Fatalf("infoInit failed to stat %s: %v", localPath, err)
	}

	info := &FileInfo{
		CID:   id,
		Name:  stat.Name(),
		IsDir: stat.IsDir(),
	}
	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		log.Fatal("infoInit failed to encode: %v", err)
	}
	os.Exit(0)
}

// infoInit chroot itself inside the container diretory, and write contents of
// the local file to stdout
func readFileInit() {
	runtime.LockOSThread()
	flag.Parse()
	// id is arg 0th
	path := flag.Arg(1)
	localPath := flag.Arg(2)

	// chroot the current process into path
	if err := syscall.Chroot(path); err != nil {
		log.Fatalf("copsInit failed to chroot to %s : %v", path, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("copsInit failed to change dir to / : %v", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	io.Copy(os.Stdout, f)
	os.Exit(0)
}

// this is run inside a chroot environment and process the actual container
// request.
func ListenAndServe(id string, r io.Reader, w io.Writer) chan error {
	decoder := json.NewDecoder(r)
	encoder := json.NewEncoder(w)
	// collection of handlers for this container.
	handlers := map[string]Handler{}

	errCh := make(chan error, 1)
	go func() {
		for {
			msg := &Message{}
			if err := decoder.Decode(msg); err != nil {
				errCh <- fmt.Errorf("failed to decode msg %s", err)
				return
			}

			// get the handler for this namespace, init and cache it if
			// necessary
			h, ok := handlers[msg.Namespace]
			if !ok {
				var err error
				newHandler, ok := Handlers[msg.Namespace]
				if !ok {
					errCh <- fmt.Errorf("invalid operation %s", msg.Namespace)
					return
				}

				h, err = newHandler(id)
				if err != nil {
					errCh <- fmt.Errorf("failed to handle %s for container %s, %s", msg.Namespace, err)
					return
				}
				handlers[msg.Namespace] = h
			}

			// handle the operation in a separate goroutine to not block the
			// message pipeline
			go func() {
				uh, ok := h.(AsyncHandler)
				if ok {
					ch := uh.HandleAsync(msg)
					for reply := range ch {
						if err := encoder.Encode(reply); err != nil {
							errCh <- fmt.Errorf("failed to encode %v, %s", reply, err)
							return
						}
					}
				} else {
					reply := h.Handle(msg)
					if err := encoder.Encode(reply); err != nil {
						errCh <- fmt.Errorf("failed to encode %v, %s", reply, err)
						return
					}
				}
			}()
		}
	}()
	return errCh
}

// Start listening to messages meant for container with `id`, spins up a
// chroot-d process for that container and proxy data in/out of the chrooted
// process.
// This is run in the parent process
func Start(id string, r io.Reader, w io.Writer) (quit chan struct{}, done chan error, err error) {
	root := filepath.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}

	parent, child, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(execCommandName, id, root)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.ExtraFiles = []*os.File{child}
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// parent doesnt need the child, so close it asap
	if err := cmd.Start(); err != nil {
		child.Close()
		return nil, nil, err
	}
	child.Close()

	done = make(chan error)
	quit = make(chan struct{})
	// write to error channel whenever the command finishes
	go func() {
		done <- cmd.Wait()
		close(done)
	}()
	// listen to quit and kill the commmand whenever it's closed
	go func() {
		<-quit
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()
	// copy data in and out of the child process
	// input goes to stdin
	// output comes from the pipe, not stdout because stdout/stderr may
	// contains logs
	go io.Copy(stdin, r)
	go io.Copy(w, parent)

	return quit, done, nil
}

// Write contents of a path inside container to the writer.
// This should be used only for file. For directory, it's best to use docker's
// built-in copy
func ReadFromContainer(id string, localPath string, w io.Writer) error {
	root := filepath.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(readFileCommandName, id, root, localPath)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Get FileInfo of a file inside the container
func GetFileInfo(id string, localPath string) (*FileInfo, error) {
	root := filepath.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	cmd := reexec.Command(infoCommandName, id, root, localPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	info := &FileInfo{}
	if err := json.Unmarshal(out, info); err != nil {
		return nil, err
	}
	return info, nil
}

func RegisterHandler(namespace string, h NewHandler) {
	Handlers[namespace] = h
}
