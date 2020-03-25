package main

import (
	"stackServer/server"
	"time"
)

func main() {
	server.Run()
	time.Sleep(time.Second * 1)
}
