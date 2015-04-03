package shellquote

import "strings"

func isSafeCmdChar(c byte) bool {
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	switch c {
	case '_', '@', '%', '+', '=', ':', ',', '.', '/', '-':
		return true
	default:
		return false
	}
}

func hasUnsafeCmdChars(s string) bool {
	for i, _ := range s {
		if !isSafeCmdChar(s[i]) {
			return true
		}
	}
	return false
}

/*
QuoteCmd is a port of python's shlex.Quote (https://github.com/python/cpython/blob/master/Lib/shlex.py#L281)
If you have a command to execute like
   ls -lah *
and you want it to execute with e.g. /bin/bash -c, you can't do:
   /bin/bash ls -lah *
You have to quote the argument and do:
   /bin/bash 'ls -lah *'
QuoteCmd() performs such quoting.
*/
func QuoteCmd(s string) string {
	if !hasUnsafeCmdChars(s) {
		return s
	}
	// use single quotes, and put single quotes into double quotes
	// the string $'b is then quoted as '$'"'"'b'
	s = strings.Replace(s, "'", "'\"'\"'", -1)
	return "'" + s + "'"
}

func isUnsafeArgChar(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func hasUnsafeArgChars(s string) bool {
	for i, _ := range s {
		if isUnsafeArgChar(s[i]) {
			return true
		}
	}
	return false
}

/*
Let's say you have a command and its arguments as an array, e.g. ["ls", "-lah", "*"]
or ["cat", "my file.txt"]. This quotes the arguments so that they can be assembled
into a command.
*/
func quoteCmdArg(s string) string {
	if !hasUnsafeArgChars(s) {
		return s
	}
	s = strings.Replace(s, `"`, `\"`, -1)
	return `"` + s + `"`
}

// CmdToString turns an array of cmd with its args into a string that can be executed
// in a shell
func CmdToString(cmdWithArgs []string) string {
	if len(cmdWithArgs) == 0 {
		return ""
	}
	if len(cmdWithArgs) == 1 {
		return cmdWithArgs[0]
	}
	args := cmdWithArgs[1:]
	res := cmdWithArgs[0]
	for _, arg := range args {
		res += " " + quoteCmdArg(arg)
	}
	return res
}
