package server

import "net"

func (s *Server) Listener() net.Listener {
	return s.listener
}
