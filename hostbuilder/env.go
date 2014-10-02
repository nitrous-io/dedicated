// this set private variables depends on environment
// mostly use for things like debug/testing etc.
package hostbuilder

import "os"

var debug bool

func init() {
	if os.Getenv("DEBUG") == "true" {
		debug = true
	}
}
