package main

import (
	"github.com/elians/websockets/server"
)

func main() {
	s := server.NewWebsocket(":5000", "/test")
	s.Listen()
}
