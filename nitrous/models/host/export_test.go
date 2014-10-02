package host

// Set variables read via envvars so tests don't rely on fixed env
func SetHostnames(root, proxy string) {
	rootHostname = root
	proxyHostname = proxy
}
