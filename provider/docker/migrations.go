package docker

func migrations() [][]string {
	return [][]string{
		{"0", `CREATE TABLE ports (port INTEGER PRIMARY KEY NOT NULL)`},
	}
}
