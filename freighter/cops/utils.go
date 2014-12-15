package cops

// if the path is not absolute, start from the current directory
func GetPath(name string) string {
	if len(name) > 0 && name[0] == '/' {
		return name
	}
	return "./" + name
}
