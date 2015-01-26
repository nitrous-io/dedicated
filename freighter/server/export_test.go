package server

func (s *Server) SetSecret(val string) {
	s.secret = val
}

func (s *Server) TerminateActiveCops(cid string) {
	s.terminateActiveCops(cid)
}
