package server

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter"
)

func (s *Server) drawRoutes() {
	// handle forwarding requests. This will only handle requests that has
	// X-FORWARDED-PORT
	s.router.NewRoute().MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) bool {
		fp := r.Header.Get(freighter.ForwardPortHeader)
		if fp == "" {
			return false
		}
		_, err := strconv.ParseInt(fp, 10, 64)
		if err != nil {
			return false
		}
		return true
	}).HandlerFunc(s.wrapError(s.httpForward))

	s.route("/", s.home, "GET")

	// advertise features
	s.route("/features", s.cors(s.features), "GET")

	s.secretRoute("/containers", s.listContainers, "GET")
	s.secretRoute("/containers", s.createContainer, "POST")
	s.secretRoute("/containers/{id}", s.getContainerInfo, "GET")
	s.secretRoute("/containers/{id}", s.terminateContainer, "DELETE")
	s.secretRoute("/containers/{id}/config", s.getContainerConfig, "GET")
	s.secretRoute("/containers/{id}/commit", s.commitContainer, "POST")
	s.secretRoute("/containers/{id}/start", s.startContainer, "POST")
	s.secretRoute("/containers/{id}/stop", s.stopContainer, "POST")
	s.secretRoute("/containers/{id}/restart", s.restartContainer, "POST")
	s.secretRoute("/containers/{id}/export", s.exportContainer, "GET")

	// FIXME: to avoid issues with nitrous/freighter not being in sync, we
	// keep the old end-points (*/public_keys) for a while and will remove
	// them when all freighter instances are upgraded to have the new */user_keys,
	// which was added mid-feb 2015
	s.secretRoute("/containers/{id}/public_keys", s.listUserKeys, "GET")
	s.secretRoute("/containers/{id}/public_keys", s.addUserKeys, "POST", "PUT")
	s.secretRoute("/containers/{id}/public_keys", s.clearUserKeys, "DELETE")
	s.secretRoute("/containers/{id}/public_keys/{key_fingerprint}", s.removeUserKey, "DELETE")
	s.secretRoute("/containers/{id}/user_keys", s.listUserKeys, "GET")
	s.secretRoute("/containers/{id}/user_keys", s.addUserKeys, "POST", "PUT")
	s.secretRoute("/containers/{id}/user_keys", s.clearUserKeys, "DELETE")
	s.secretRoute("/containers/{id}/user_keys/{key_fingerprint}", s.removeUserKey, "DELETE")
	s.secretRoute("/containers/{id}/container_keys", s.listContainerKeys, "GET")
	s.secretRoute("/containers/{id}/reconfig", s.reconfigureContainer, "POST")

	// handle websocket custom protocol
	s.route("/containers/{id}/shell", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/shell", s.cors(s.wrapError(s.withContainerToken(s.wsShell, "shell"))), "GET")
	s.route("/containers/{id}/logs", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/logs", s.cors(s.wrapError(s.withContainerToken(s.wsContainerLogs, "logs"))), "GET")
	s.route("/containers/{id}/attach", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/attach", s.cors(s.wrapError(s.withContainerToken(s.wsAttachToContainer, "attach"))), "GET")

	// handle container operations
	s.route("/containers/{id}/ops", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/ops", s.cors(s.wrapError(s.withContainerToken(s.wsContainerOps, "ops"))), "GET")

	// handle collab
	s.route("/containers/{id}/collab", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/collab", s.cors(s.wrapError(s.withContainerToken(s.wsCollab, "collab"))), "GET")

	// handle download/upload files to a container
	s.route("/containers/{id}/download", s.wrapError(s.withContainerToken(s.downloadContainerFile, "download")), "GET")
	s.route("/containers/{id}/upload", s.handleCORSPreflight, "OPTIONS")
	s.route("/containers/{id}/upload", s.cors(limitRequestSize(
		s.wrapError(s.withContainerToken(s.uploadContainerFile, "upload")),
		MaxRequestSize)), "POST")

	s.secretRoute("/registry/auth", s.loginRegistry, "POST")
	s.secretRoute("/registry/auth", s.logoutRegistry, "DELETE")

	s.secretRoute("/images", s.wrapError(s.listImages), "GET")
	s.secretRoute("/images", s.wrapError(s.buildImage), "POST")
	s.secretRoute("/images", s.wrapError(s.removeImage), "DELETE")
	s.secretRoute("/images/pull", s.wrapError(s.pullImage), "POST")
	s.secretRoute("/images/push", s.wrapError(s.pushImage), "POST")
	s.secretRoute("/images/tag", s.wrapError(s.tagImage), "POST")
	s.secretRoute("/images/inspect", s.wrapError(s.inspectImage), "GET")

	s.secretRoute("/volumes", s.wrapError(s.listVolumes), "GET")
	s.secretRoute("/volumes", s.wrapError(s.createVolume), "POST")
	s.secretRoute("/volumes/{slug}", s.wrapError(s.removeVolume), "DELETE")
	s.secretRoute("/volumes/{slug}", s.wrapError(s.getVolumeInfo), "GET")

	s.secretRoute("/info", s.wrapError(s.serverInfo), "GET")
	s.secretRoute("/versions", s.wrapError(s.serverVersions), "GET")
	s.secretRoute("/utilization", s.wrapError(s.serverUtilization), "GET")

	s.secretRoute("/upgrade", s.wrapError(s.upgradeServer), "POST")
	s.secretRoute("/upgradefreighter", s.wrapError(s.upgradeFreighter), "POST")
}
