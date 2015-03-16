package hostbuilder

func (s *Server) ProviderConfig(provider string) map[string]string {
	return s.providerConfig(provider)
}
