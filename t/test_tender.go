package main

import "fmt"

/*
This orchestrates testing of tender by starting up all processes
that need to be started and running test logic.

On failure does os.Exit(1).
*/

// forego run -e t/.env-tender build/tender

func main() {
	fmt.Printf("this is tender loving\n")
}
