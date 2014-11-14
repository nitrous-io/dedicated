package server

import "net"

var GetRemoteAddr = getRemoteAddr

func (s *Server) Listener() net.Listener {
	return s.listener
}
