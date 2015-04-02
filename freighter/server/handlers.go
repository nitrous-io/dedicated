package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/nitrous-io/dedicated/freighter"
	"github.com/nitrous-io/dedicated/freighter/cops"
	"github.com/nitrous-io/dedicated/freighter/pkg/flusher"
	"github.com/nitrous-io/dedicated/freighter/pkg/shell"
	"github.com/nitrous-io/dedicated/freighter/pkg/stats"
	"github.com/nitrous-io/dedicated/freighter/provider"
	proxyerror "github.com/nitrous-io/dedicated/pkg/errorpages/proxy"
	"github.com/nitrous-io/dedicated/pkg/msgproto"
	"github.com/nitrous-io/dedicated/pkg/multipartsio"
	"github.com/nitrous-io/dedicated/pkg/proxy"
	"github.com/nitrous-io/nitrous-common/wsconn"
)

var (
	defaultUploadMemory int64 = 20 << 20 // 20MB
)

const StatusUnprocessableEntity = 422

// default size of the shell's message queue
const messageQueueSize = 10

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// allow all origins here
	CheckOrigin: func(r *http.Request) bool { return true },
}

const utilizationInterval = 5 * time.Second
const utilizationSize = 360 // 30min

var (
	Utilization *stats.Utilization

	ActiveCops *ActiveCopsList
)

// maps container id to slice of 'quit' chan error returned by cops.Start
type ActiveCopsList struct {
	mu         sync.Mutex
	activeCops map[string][]chan error
}

func NewActiveCopsList() *ActiveCopsList {
	return &ActiveCopsList{
		activeCops: make(map[string][]chan error),
	}
}

func (ac *ActiveCopsList) Len(cid string) int {
	ac.mu.Lock()
	res := len(ac.activeCops[cid])
	ac.mu.Unlock()
	return res
}

func (ac *ActiveCopsList) Terminate(cid string) {
	ac.mu.Lock()
	for _, quit := range ac.activeCops[cid] {
		quit <- nil
	}
	ac.mu.Unlock()
}

func (ac *ActiveCopsList) AddChannel(cid string, ch chan error) {
	ac.mu.Lock()
	ac.activeCops[cid] = append(ac.activeCops[cid], ch)
	ac.mu.Unlock()
}

func (ac *ActiveCopsList) RemoveChannel(cid string, toRemove chan error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for i, ch := range ac.activeCops[cid] {
		if ch == toRemove {
			ac.activeCops[cid] = append(ac.activeCops[cid][:i], ac.activeCops[cid][i+1:]...)
			return
		}
	}
}

func NewWebSocketConnection(conn *websocket.Conn, binaryMode bool) *wsconn.Conn {
	stream := wsconn.NewConnectionHeartbeat(conn, true)
	if binaryMode {
		stream.MessageType = websocket.BinaryMessage
	}

	return stream
}

func init() {
	Utilization = stats.NewUtilization(utilizationSize)
	go Utilization.Record(utilizationInterval)

	ActiveCops = NewActiveCopsList()
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusOK, jsonStatusOK)
}

func (s *Server) handleCORSPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	if h := r.Header["Access-Control-Request-Headers"]; h != nil {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(h, ", "))
	}
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	s.respond(w, http.StatusOK, jsonStatusOK)
}

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	list, err := s.provider.List()
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"containers": list})
}

// FIXME This handler has no existing test :"(
func (s *Server) reconfigureContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	s.terminateActiveCops(id)
	info, err := s.provider.Reconfig(id, r.FormValue("config"))
	if err != nil {
		s.logger.Error(err)

		if info != nil {
			s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{
				"container": info,
				"status":    "reconfig_failed",
				"error":     err.Error(),
			})
			return
		}

		s.respondEncodeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"container": info})
}

func (s *Server) createContainer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	image := r.FormValue("image")
	secret := r.FormValue("secret")

	if name == "" {
		s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"name is required"}`))
		return
	} else if !provider.IsValidName(name) {
		s.respondEncodeJSON(w, StatusUnprocessableEntity,
			map[string]interface{}{"error": fmt.Sprintf("name %q is not valid", name)})
		return
	} else if image == "" {
		s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"image is required"}`))
		return
	} else if secret == "" {
		s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"secret is required"}`))
		return
	}

	list, err := s.provider.List()
	if err != nil {
		s.handleError(w, err)
		return
	}
	for _, c := range list {
		if c.Name == name {
			s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"name is already taken"}`))
			return
		}
	}

	info, err := s.provider.CreateStart(name, image, secret, r.FormValue("config"))
	_, startFailed := err.(*provider.StartFailedError)
	switch {
	case err == nil:
		s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"container": info})
	case startFailed:
		s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{
			"container": info,
			"status":    "start_failed",
			"error":     err.Error(),
		})
	case err == provider.ErrImageNotFound:
		s.respondEncodeJSON(w, http.StatusNotFound, map[string]interface{}{"error": err.Error()})
	default:
		s.respondEncodeJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
	}
}

func (s *Server) commitContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	name := r.FormValue("name")
	if name == "" {
		s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"name is missing"}`))
		return
	}

	image, err := s.provider.Commit(id, name)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"image": image})
}

func (s *Server) getContainerInfo(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	info, err := s.provider.GetInfo(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"container": info})
}

func (s *Server) getContainerConfig(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	config, err := s.provider.GetConfig(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{
		"config": config,
	})
}

func (s *Server) terminateContainer(w http.ResponseWriter, r *http.Request) {
	s.performContainerControl(w, r,
		func(id string) (*provider.ContainerInfo, error) {
			s.terminateActiveCops(id)
			return s.provider.Terminate(id, true)
		},
		"terminated")
}

func (s *Server) startContainer(w http.ResponseWriter, r *http.Request) {
	config := r.FormValue("config")
	curryStart := func(id string) (*provider.ContainerInfo, error) {
		return s.provider.Start(id, config)
	}

	s.performContainerControl(w, r, curryStart, "started")
}

func (s *Server) stopContainer(w http.ResponseWriter, r *http.Request) {
	s.performContainerControl(w, r,
		func(id string) (*provider.ContainerInfo, error) {
			s.terminateActiveCops(id)
			return s.provider.Stop(id)
		},
		"stopped")
}

func (s *Server) restartContainer(w http.ResponseWriter, r *http.Request) {
	s.performContainerControl(w, r,
		func(id string) (*provider.ContainerInfo, error) {
			s.terminateActiveCops(id)
			return s.provider.Restart(id)
		},
		"restarted")
}

func (s *Server) listUserKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	allPubKeys, err := s.provider.ListUserKeys(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (s *Server) addUserKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	err := r.ParseForm()
	if err != nil {
		s.handleError(w, err)
		return
	}

	publicKeys := r.PostForm["public_key[]"]

	if publicKeys == nil {
		s.respond(w, StatusUnprocessableEntity, []byte(`{"error":"public_key[] is missing"}`))
		return
	}

	err = s.provider.AddUserKeys(id, publicKeys)
	if err != nil {
		if err == provider.ErrPubKeyOpsNotSupported {
			s.respondEncodeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		} else {
			s.handleError(w, err)
		}

		return
	}

	allPubKeys, err := s.provider.ListUserKeys(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (s *Server) removeUserKey(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	keyFingerprint := mux.Vars(r)["key_fingerprint"]

	err := s.provider.RemoveUserKey(id, keyFingerprint)
	if err != nil {
		s.handleError(w, err)
		return
	}

	allPubKeys, err := s.provider.ListUserKeys(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"public_keys": allPubKeys})
}

func (s *Server) clearUserKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	err := s.provider.ClearUserKeys(id)
	if err != nil {
		s.handleError(w, err)
		return
	}

	s.respond(w, http.StatusOK, []byte{})
}

func (s *Server) listContainerKeys(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	allPubKeys, err := s.provider.ListContainerKeys(id)
	if err != nil {
		switch err {
		case provider.ErrContainerNotRunning:
			s.respond(w, http.StatusConflict, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		case provider.ErrPubKeyOpsNotSupported:
			s.respond(w, http.StatusBadRequest, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		default:
			s.handleError(w, err)
		}
		return
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"container_keys": allPubKeys})
}

func (s *Server) createVolume(w http.ResponseWriter, r *http.Request) error {
	slug := r.FormValue("slug")
	vi, err := s.provider.CreateVolume(slug)
	if err != nil {
		// temporary workaround until we start using httperror
		if strings.HasPrefix(err.Error(), "volume") {
			s.respond(w, 422, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
			return nil
		}
		return err
	}
	s.respondEncodeJSON(w, http.StatusCreated, map[string]interface{}{"volume": vi})
	return nil
}

func (s *Server) listVolumes(w http.ResponseWriter, r *http.Request) error {
	volumes, err := s.provider.ListVolumes()
	if err != nil {
		return err
	}
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"volumes": volumes})
	return nil
}

func (s *Server) getVolumeInfo(w http.ResponseWriter, r *http.Request) error {
	slug := mux.Vars(r)["slug"]
	vi, err := s.provider.GetVolumeInfo(slug)
	if err != nil {
		// temporary workaround until we start using httperror
		if strings.HasPrefix(err.Error(), "volume") {
			s.respond(w, 422, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
			return nil
		}
		return err
	}
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"volume": vi})
	return nil
}

func (s *Server) removeVolume(w http.ResponseWriter, r *http.Request) error {
	slug := mux.Vars(r)["slug"]
	err := s.provider.RemoveVolume(slug)
	if err != nil {
		// temporary workaround until we start using httperror
		if strings.HasPrefix(err.Error(), "volume") {
			s.respond(w, 422, []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
			return nil
		}
		return err
	}
	s.respond(w, http.StatusOK, jsonStatusOK)
	return nil
}

// export content of the container as a .tar stream
func (s *Server) exportContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	w.Header().Set("Content-Type", "application/octet-stream")
	err := s.provider.Export(id, w)
	if err != nil {
		s.handleError(w, err)
	}
}

func (s *Server) wsContainerLogs(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]

	// convert to bool, ignore the errors as the value will be false when error
	// happens
	follow, _ := strconv.ParseBool(r.FormValue("follow"))
	timestamps, _ := strconv.ParseBool(r.FormValue("timestamps"))
	binary, _ := strconv.ParseBool(r.FormValue("binary"))
	tail := r.FormValue("tail")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("failed to upgrade websocket: %v", err)
	}
	stream := NewWebSocketConnection(conn, binary)
	defer stream.Close()

	return s.provider.Logs(id, shell.NewShellWriter(stream), follow, timestamps, tail)
}

// wsNewShell creates a new shell to a container using protocol specified in
// pkg/shell/protocol.go
func (s *Server) wsNewShell(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]

	info, err := s.provider.GetInfo(id)
	if err != nil {
		return err
	}
	containerID := info.ID
	if info.State != provider.ContainerStateRunning {
		return fmt.Errorf("Cannot attach to stopped container %v", containerID)
	}

	command := r.FormValue("command")
	width := getFormInt(r, "width")
	height := getFormInt(r, "height")
	// check if the client request to use BinaryMessage instead of TextMessage
	binary, _ := strconv.ParseBool(r.FormValue("binary"))
	// check if the client wants to have a TTY. Usually the client who wants a
	// stdin pipe will not need a TTY
	noTTY, _ := strconv.ParseBool(r.FormValue("notty"))

	sh, err := shell.OpenShell(containerID, command, noTTY)
	if err != nil {
		fmt.Printf("wsNewShell: failed to open a shell for container %v, err: %s\n", containerID, err)
		return err
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("failed to upgrade websocket: %v", err)
	}
	stream := NewWebSocketConnection(conn, binary)
	defer stream.Close()

	// given the shell an ID so we can multiplex the shell IO later given its
	// id i.e. gomux style
	shellID := randomString()
	s.shells[shellID] = sh

	// create stdin/stdout stream with the shell protocol
	msgCh := make(chan shell.Message, messageQueueSize)
	stdin := shell.NewShellReader(conn, msgCh, stream.MessageType)
	stdout := shell.NewShellWriter(stream)

	attached := make(chan struct{}, 1)
	ch := sh.Attach(stdin, stdout, attached)

	for {
		select {
		case err := <-ch:
			log.Printf("exited with err %v", err)
			stdout.Close() // Inform client that server has closed connection.
			return nil
		case <-attached:
			// when the client first attached, resize immediately with the
			// current width and height
			sh.Resize(width, height)
		case msg := <-msgCh:
			switch msg := msg.(type) {
			case *shell.ResizeMsg:
				sh.Resize(msg.Width, msg.Height)
			default:
				log.Println("shell: got unknown msg ", msg)
			}
		}
	}
}

// returns true if a given env variable should be passed to cops process
// envVar is in FOO=bar format
func copsWantedEnvVar(envVar string) bool {
	parts := strings.SplitN(envVar, "=", 2)
	if len(parts) != 2 {
		return false
	}
	switch parts[0] {
	case "__HOST", "NITROUS_IMAGE_VERSION", "NITROUS_USERNAME", "NITROUS_SSH_ENABLED":
		return true
	default:
		return false
	}
}

func copsAdditionalEnv(env []string) []string {
	var res []string
	for _, e := range env {
		if copsWantedEnvVar(e) {
			res = append(res, e)
		}
	}
	return res
}

func (s *Server) wsContainerOps(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	// check if the client request to use BinaryMessage instead of TextMessage
	binary, _ := strconv.ParseBool(r.FormValue("binary"))

	// get environment variables from docker so that we can pass them to cops process
	info, err := s.provider.GetInfo(id)
	if err != nil {
		return err
	}
	if info.State != provider.ContainerStateRunning {
		return fmt.Errorf("container ops: cannot perform container ops on stopped container %v", info.ID)
	}
	copsEnv := copsAdditionalEnv(info.Env)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("container ops: failed to upgrade websocket: %v", err)
	}
	ws := NewWebSocketConnection(conn, binary)
	defer ws.Close()

	cmd, copsStdin, copsStdout, err := cops.Start(id, copsEnv)
	if err != nil {
		return err
	}

	msgsFromCops := make(chan map[string]interface{})
	quit := make(chan error, 2)

	// read json messages from cops process
	go func() {
		dec := json.NewDecoder(copsStdout)
		for {
			var msg map[string]interface{}
			if err := dec.Decode(&msg); err == nil {
				msgsFromCops <- msg
			} else {
				log.Printf("wsContainerOps: dec.Decode() failed with %s", err)
				quit <- err
				return
			}
		}
	}()

	// read ws message from client and send them to cops process
	// the assumption is that messages are json, which serves as a message
	// framing mechanism (json.Decoder handles such stream of json messages)
	go func() {
		var err error
		var msgType int
		var d []byte
		for {
			msgType, d, err = msgproto.ReadAndDecodeMessage(ws)
			if err != nil {
				log.Printf("wsContainerOps: msgproto.ReadAndDecodeMessage() failed with %s", err)
				break
			}
			if msgType == msgproto.CloseMessage {
				log.Printf("wsContainerOps: msgproto.ReadAndDecodeMessage() received msgproto.CloseMessge")
				break
			}
			if _, err = copsStdin.Write(d); err != nil {
				log.Printf("cops.Start(): stdin.Write() failed with %s", err)
				break
			}
		}
		quit <- err
	}()

	// wait for process to exit
	go func() {
		err := cmd.Wait()
		quit <- err
	}()

	ActiveCops.AddChannel(id, quit)
	for {
		select {
		case err := <-quit:
			log.Printf("container ops: exited with err %v", err)
			ActiveCops.RemoveChannel(id, quit)
			ws.Close() // inform client that server has closed connection.
			if cmd.Process != nil {
				log.Printf("wsContainerOps: killing cops process")
				cmd.Process.Kill()
			}
			return nil

		case msg := <-msgsFromCops:
			d, err := json.Marshal(msg)
			if err != nil {
				log.Printf("json.Marshal() of %v failed with %s\n", msg, err)
			} else {
				_, err = msgproto.WriteDataMessage(d, ws)
				if err != nil {
					log.Printf("msgproto.WriteDataMessage() failed with '%s'\n", err)
				}
			}

			if err != nil {
				quit <- err
			}
		}
	}
}

func (s *Server) loginRegistry(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	email := r.FormValue("email")
	registry := r.FormValue("registry")

	status := http.StatusCreated
	err := s.provider.Login(username, password, email, registry)
	if err != nil {
		status = StatusUnprocessableEntity
	}
	s.respond(w, status, nil)
}

func (s *Server) logoutRegistry(w http.ResponseWriter, r *http.Request) {
	registry := r.FormValue("registry")

	status := http.StatusNoContent
	err := s.provider.Logout(registry)
	if err != nil {
		status = StatusUnprocessableEntity
	}
	s.respond(w, status, nil)
}

var defaultPeriod = 100 * time.Millisecond

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) error {
	period, err := time.ParseDuration(r.FormValue("period"))
	if err != nil || period == 0 {
		period = defaultPeriod
	}
	stats, err := stats.Sample(period)
	if err != nil {
		return err
	}
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"info": stats})
	return nil
}

var (
	dockerHost = "unix:///var/run/docker.sock"
)

// /versions
func (s *Server) serverVersions(w http.ResponseWriter, r *http.Request) error {
	v := map[string]interface{}{
		"freighterd_version": freighter.Version,
	}
	if dockerVer, err := s.provider.Version(); err == nil {
		v["docker_version"] = dockerVer
	}
	s.respondEncodeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) serverUtilization(w http.ResponseWriter, r *http.Request) error {
	utilization := Utilization.Fetch()
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"utilization": utilization})
	return nil
}

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) error {
	images, err := s.provider.ListImages()
	if err != nil {
		return err
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"images": images})
	return nil
}

func (s *Server) buildImage(w http.ResponseWriter, r *http.Request) error {
	outstream := flusher.NewWriteFlusher(w)
	opts := provider.BuildImageOptions{
		Name:          r.FormValue("name"),
		OutputStream:  outstream,
		RawJSONStream: true,
	}

	if remote := r.FormValue("remote"); remote != "" {
		opts.Remote = remote
	} else {
		_, fh, err := r.FormFile("file")
		if err != nil {
			return err
		}

		f, err := fh.Open()
		if err != nil {
			return err
		}
		defer f.Close()

		opts.InputStream = f
	}

	if err := s.provider.BuildImage(opts); err != nil {
		return err
	}
	return nil
}

func (s *Server) removeImage(w http.ResponseWriter, r *http.Request) error {
	image := r.FormValue("image")
	err := s.provider.RemoveImage(image)
	if err != nil {
		if err == provider.ErrImageInUse {
			s.respondEncodeJSON(w, http.StatusConflict, map[string]interface{}{"error": err.Error()})
			return nil
		}
		return err
	}
	s.respond(w, http.StatusOK, jsonStatusOK)
	return nil
}

// pullImage outputs raw docker's raw json stream to the client so it can
// format the download progress by itself
//
// Refers to // https://docs.docker.com/reference/api/docker_remote_api_v1.13/#22-images
// for more information on the output format
func (s *Server) pullImage(w http.ResponseWriter, r *http.Request) error {
	update, err := strconv.ParseBool(r.FormValue("update"))
	if err != nil {
		update = true
	}
	repo := r.FormValue("repository")

	if !update {
		// Don't update if image already exists.
		if _, err := s.provider.InspectImage(repo, false); err == nil {
			s.respondEncodeJSON(w, http.StatusNotModified, nil)
			return nil
		}
	}

	outstream := flusher.NewWriteFlusher(w)
	if err := s.provider.PullImage(repo, outstream, true); err != nil {
		return err
	}
	return nil
}

func (s *Server) pushImage(w http.ResponseWriter, r *http.Request) error {
	image := r.FormValue("image")
	destination := r.FormValue("destination")

	outstream := flusher.NewWriteFlusher(w)
	if err := s.provider.PushImage(image, destination, outstream, true); err != nil {
		if err == provider.ErrImagePushAuthRequired {
			s.respondEncodeJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": err.Error()})
			return nil
		}
		return err
	}
	return nil
}

func (s *Server) tagImage(w http.ResponseWriter, r *http.Request) error {
	image := r.FormValue("image")
	tag := r.FormValue("tag")
	force, _ := strconv.ParseBool(r.FormValue("force"))
	err := s.provider.TagImage(image, tag, force)
	if err != nil {
		return err
	}
	s.respond(w, http.StatusOK, jsonStatusOK)
	return nil
}

func (s *Server) inspectImage(w http.ResponseWriter, r *http.Request) error {
	image := r.FormValue("image")
	if image == "" {
		s.respondEncodeJSON(w, StatusUnprocessableEntity, map[string]interface{}{
			"error": "image is not valid",
		})
		return nil
	}
	remote, err := strconv.ParseBool(r.FormValue("remote"))
	if err != nil {
		remote = true
	}
	info, err := s.provider.InspectImage(image, remote)
	if err != nil {
		return err
	}
	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"image": info})
	return nil
}

func (s *Server) wsAttachToContainer(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	info, err := s.provider.GetInfo(id)
	if err != nil {
		return err
	}
	if info.State != provider.ContainerStateRunning {
		return fmt.Errorf("Cannot attach to stopped container %v", info.ID)
	}

	// check if the client request to use BinaryMessage instead of TextMessage
	binary, _ := strconv.ParseBool(r.FormValue("binary"))
	// if no-stdin in param is set, STDIN should not be attached
	noStdin, _ := strconv.ParseBool(r.FormValue("nostdin"))

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("failed to upgrade websocket: %v", err)
	}
	stream := NewWebSocketConnection(conn, binary)
	defer stream.Close()

	// create stdin/stdout stream with the shell protocol
	msgCh := make(chan shell.Message, messageQueueSize)
	stdin := shell.NewShellReader(conn, msgCh, stream.MessageType)
	stdout := shell.NewShellWriter(stream)
	exit := make(chan struct{})

	go func() {
		defer close(exit)
		if err := s.provider.AttachToContainer(id, stdin, stdout, noStdin); err != nil {
			// Connection is already hijacked,
			// log the error and close the connection
			s.logger.Errorf("attach: %s", err)
			stream.Write([]byte("c|an error occurred"))
		}
	}()

	for {
		select {
		case <-exit:
			return nil
		case msg := <-msgCh:
			switch msg := msg.(type) {
			case *shell.ResizeMsg:
				err = s.provider.ResizeContainerTTY(id, msg.Height, msg.Width)
			default:
				s.logger.Info("attach: got unknown msg %v", msg)
			}
		}
	}
}

func (s *Server) downloadContainerFile(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	info, err := s.provider.GetInfo(id)
	if err != nil {
		return err
	}
	if info.State != provider.ContainerStateRunning {
		return fmt.Errorf("Cannot attach to stopped container %v", info.ID)
	}

	localPath := r.FormValue("path")
	fileName := filepath.Base(localPath)
	stat, err := s.provider.FileInfo(id, localPath)
	if err != nil {
		return err
	}
	if stat.IsDir {
		fileName = fileName + ".tar"
	}
	w.Header().Set("Content-Disposition", contentDisposition(fileName))
	return s.provider.CopyFile(id, localPath, w)
}

func (s *Server) uploadContainerFile(w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	info, err := s.provider.GetInfo(id)
	if err != nil {
		return err
	}
	if info.State != provider.ContainerStateRunning {
		return fmt.Errorf("Cannot attach to stopped container %v", info.ID)
	}
	if err := r.ParseMultipartForm(defaultUploadMemory); err != nil {
		return err
	}
	defer r.MultipartForm.RemoveAll()

	location := r.FormValue("location")
	input := make(map[string]io.Reader)
	for name, fhs := range r.MultipartForm.File {
		// we only fetch the fisrt file for each key similar to how FormFile works
		if len(fhs) > 0 {
			part := fhs[0]
			if part.Filename != "" {
				name = part.Filename
			}
			input[name] = multipartsio.NewPartsReader(part)
		}
	}

	if err := s.provider.CopyFromHost(info.ID, location, input); err != nil {
		return err
	}

	s.respondEncodeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
	return nil
}

func (s *Server) httpForward(w http.ResponseWriter, r *http.Request) error {
	if r.TLS != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	}

	addr := fmt.Sprintf("%s:%s", "0.0.0.0", r.Header.Get(freighter.ForwardPortHeader))
	if proxy.IsWebsocket(r) {
		proxy.ProxyWebsocket(addr, nil)(w, r)
	} else {
		proxy.ProxyHTTP(addr, nil, proxyerror.ErrorHandler)(w, r)
	}
	return nil
}

// upgradeServer upgrades the freighterd and/or docker package to the version
// specified in the request. This assumes that we are using APT to manage
// freighterd and will probably not work everywhere.
// sample docker_version: "1.3.0"
// sample freighterd_version: "0.2.0+1424818140-0-0b52a6e"
func (s *Server) upgradeServer(w http.ResponseWriter, r *http.Request) error {
	fv := r.PostFormValue("freighterd_version")
	dv := r.PostFormValue("docker_version")
	myDockerVer, err := s.provider.Version()
	if err != nil {
		return err
	}

	upgradingDocker := dv != "" && dv != myDockerVer
	upgradingFreighter := fv != "" && fv != freighter.Version

	upgradingAny := upgradingDocker || upgradingFreighter
	if !upgradingAny {
		s.respondEncodeJSON(w, http.StatusNoContent, nil)
		return nil
	}

	if !upgradingDocker {
		dv = ""
	}
	if !upgradingFreighter {
		fv = ""
	}

	go s.upgradeDockerAndFreighter(dv, fv)

	s.respond(w, http.StatusAccepted, jsonStatusAccepted)
	return nil
}

func (s *Server) notifyWebhookUpgradeFailed(upgradeErr error, component string) {
	s.logger.Errorf("error upgrading freighterd and/or docker: %v", upgradeErr)
	err := s.notifyWebhook(map[string]interface{}{
		"event": "upgrade",
		"error": upgradeErr.Error(),
	})
	if err != nil {
		s.logger.Errorf("failed to notify webhook about %s update, err: %v", component, err)
	}
}

// - upgrade freighter if fv != ""
// - upgrade docker if dv != ""
// - restart freighter to notify the upgrade happened
func (s *Server) upgradeDockerAndFreighter(dv string, fv string) {
	err := s.updateAptSource()
	if err != nil {
		s.notifyWebhookUpgradeFailed(err, "docker")
		return
	}
	// if both, upgrade docker first
	if dv != "" {
		if err := s.upgradeDocker(dv); err != nil {
			s.notifyWebhookUpgradeFailed(err, "docker")
			return
		}
		s.logger.Infof("updated docker package to %q", dv)
	}
	if fv != "" {
		if err := s.upgradeFreighterd(fv); err != nil {
			s.notifyWebhookUpgradeFailed(err, "freighterd")
			return
		}
		s.logger.Infof("updated freighterd package from %q to %q", freighter.Version, fv)
	}

	if err := s.stopFreighterd(); err != nil {
		s.logger.Errorf("error stopping freighterd: %v", err)
		s.notifyWebhookUpgradeFailed(err, "freighterd")
	}
}

func (s *Server) updateAptSource() error {
	cmd := exec.Command("apt-get", "update",
		"-o", "Dir::Etc::sourceparts=-",
		"-o", "Dir::Etc::sourcelist=/etc/apt/sources.list.d/nitrousio.list")
	cmd.Env = append(cmd.Env, "DEBIAN_FRONTEND=noninteractive")
	if out, err := s.runner.Run(cmd); err != nil {
		s.logger.Errorf("error updating nitrous repo, err: %v, output: %s", err, out)
		return err
	}
	return nil
}

func (s *Server) upgradeDocker(version string) error {
	pkgspec := "lxc-docker-" + version
	cmd := exec.Command("apt-get", "install", "-y",
		"-o", `Dpkg::Options::=--force-confask`,
		"-o", `Dpkg::Options::=--force-confnew`, // confask+confnew = always replace existing conf
		pkgspec,
	)
	cmd.Env = append(cmd.Env, "DEBIAN_FRONTEND=noninteractive")
	cmd.Env = append(cmd.Env, "PATH=/usr/sbin:/usr/bin:/sbin:/bin")
	if out, err := s.runner.Run(cmd); err != nil {
		s.logger.Errorf("error updating docker package, err: %v, output: %s", err, out)
		return err
	}
	return nil
}

func (s *Server) upgradeFreighterd(version string) error {
	// Setup a dir with service-related binaries pointing to /bin/true. We will
	// use this in the PATH for the `apt-get install` command so that the
	// freighterd service is not restarted.
	// See http://askubuntu.com/a/77530/25214.
	fakepath, err := ioutil.TempDir("", "freighter-fakepath")
	if err != nil {
		return err
	}
	defer os.RemoveAll(fakepath)
	if err := setupFakePath(fakepath); err != nil {
		return err
	}

	pkgspec := "freighterd"
	if version != "" {
		pkgspec = fmt.Sprintf("freighterd=%s", version)
	}
	cmd := exec.Command("apt-get", "install", "-y",
		"-o", `Dpkg::Options::=--force-confdef`, // avoid prompt
		"-o", `Dpkg::Options::=--force-confold`, // do not modify existing config file
		pkgspec,
	)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s:/usr/sbin:/usr/bin:/sbin:/bin", fakepath))
	cmd.Env = append(cmd.Env, "DEBIAN_FRONTEND=noninteractive")
	if out, err := s.runner.Run(cmd); err != nil {
		s.logger.Errorf("error updating freighterd package, err: %v, output: %s", err, out)
		return err
	}

	return nil
}

func setupFakePath(path string) error {
	cmds := []string{"initctl", "invoke-rc.d",
		"restart", "start", "stop", "start-stop-daemon", "service"}
	for _, cmd := range cmds {
		if err := os.Symlink("/bin/true", filepath.Join(path, cmd)); err != nil {
			return err
		}
	}
	return nil
}

// Stop freighterd (i.e. this process) by sending it a signal. We rely on an
// external process (currently Upstart) to respawn the freighterd service.
// TODO We probably want to stop gracefully (maybe by sending SIGHUP to self)
// so that existing HTTP requests are not dropped.
func (s *Server) stopFreighterd() error {
	s.logger.Infof("Sending freighter process signal to terminate...")
	return syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

func randomString() string {
	id := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(id)
}

func getFormInt(r *http.Request, k string) int {
	v, err := strconv.Atoi(r.FormValue(k))
	if err != nil {
		return 0
	}
	return v
}

func contentDisposition(filename string) string {
	res := "attachment"
	if filename != "" {
		uriName := url.QueryEscape(filename)
		res = fmt.Sprintf("attachment; filename='%s'; filename*=UTF-8''%s",
			uriName, uriName)
	}
	return res
}
