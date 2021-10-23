package main

import (
	"heckel.io/notifyme/server"
	"log"
)

func main() {
	s := server.New()
	if err := s.Run(); err != nil {
		log.Fatalln(err)
	}
}
