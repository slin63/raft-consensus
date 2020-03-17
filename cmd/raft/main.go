package main

import (
	"log"
	"os"
	"strconv"

	"github.com/slin63/raft-consensus/internal/client"
	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/node"
)

func main() {
	log.SetPrefix(config.C.Prefix + " - ")
	isClient, err := strconv.ParseBool(os.Getenv("CLIENT"))
	if err != nil {
		isClient = false
	}

	if isClient {
		return node.Live()
	}
	return client.PutEntry(os.Args[1:])
}
