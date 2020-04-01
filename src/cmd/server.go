package main

import (
	"os"
	"os/signal"
	"stackServer/src/server"
)

func main() {
	stackserver := server.StackServer{}
	stackserver.Run()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
