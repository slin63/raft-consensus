package main

import (
	"log"
	"os"
	"strconv"

	"./client"
	"./config"
	"./node"
)

func main() {
	log.SetPrefix(config.C.Prefix + " - ")
	leader, err := strconv.ParseBool(os.Getenv("LEADER"))
	if err != nil {
		log.Fatal("LEADER not set in this environment")
	}
	isClient, err := strconv.ParseBool(os.Getenv("CLIENT"))
	if err != nil {
		log.Fatal("CLIENT not set in this environment")
	}

	if !isClient {
		node.Live(leader)
	} else {
		client.PutEntry(os.Args[1:])
	}
}
