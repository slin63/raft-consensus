// Bare-bones RPC client that sends over arbitrary strings
package client

import (
	"log"
	"net/rpc"
	"strings"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/pkg/responses"
)

const helpS = `Send over a string.`

var server string = "localhost:" + config.C.RPCPort

func PutEntry(args []string) {
	entry := strings.Join(args, " ")
	log.Printf(entry)

	client, err := rpc.DialHTTP("tcp", server)
	if err != nil {
		log.Fatal("[ERROR] PutEntry() dialing:", err)
	}

	// PID of assigned server
	var result *responses.Result
	if err = client.Call("Ocean.PutEntry", entry, &result); err != nil {
		log.Fatal(err)
	}
}
