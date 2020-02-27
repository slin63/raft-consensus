package main

import (
	"log"
	"os"
	"strconv"

	"./config"
	"./node"
)

func main() {
	log.SetPrefix(config.C.Prefix + " - ")
	leader, err := strconv.ParseBool(os.Getenv("LEADER"))
	if err != nil {
		log.Fatal("LEADER not set in this environment")
	}
	node.Live(leader)
}
