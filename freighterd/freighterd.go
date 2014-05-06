package main

import (
	"fmt"
	"github.com/nitrous-io/freighter/server"
	"log"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Fatalf("freighterd: Fatal Error - %v", r)
		}
	}()

	s := &server.Server{}
	host := "0.0.0.0"
	port := uint16(3342)
	fmt.Printf("freighterd listening on %s:%d\n", host, port)
	s.Start(host, port)
}
