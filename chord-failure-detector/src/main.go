package main

import (
	"flag"
	"log"
	"os"
	"strconv"

	"./node"
)

const logf = "o.log"
const prefix = "[MEMBER] - "

func main() {
	log.SetPrefix(prefix)
	isIntroducer, err := strconv.ParseBool(os.Getenv("INTRODUCER"))
	if err != nil {
		log.Fatal("INTRODUCER not set in this environment")
	}
	flag.Parse()

	node.Live(isIntroducer, logf)
}
