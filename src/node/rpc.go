// Client and server stubs for RPCs.
package node

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"

	"../config"
	"../spec"
)

// Because rafts float in the ocean
type Ocean int

func serveOceanRPC() {
	oc := new(Ocean)
	rpc.Register(oc)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":"+config.C.RPCPort)
	if e != nil {
		log.Fatal("[ERROR] serveOceanRPC():", e)
	}
	log.Println("[RPC] serveOceanRPCs")
	http.Serve(l, nil)
}

// AppendEntries (client)
// Invoked by leader to replicate log entries (§5.3); also used as heartbeat (§5.2).
// TODO (02/27 @ 11:27): only does heartbeats for now
func CallAppendEntries(PID int, args *spec.AppendEntriesArgs, wg *sync.WaitGroup) spec.Result {
	defer wg.Done()
	client := connect(PID)
	defer client.Close()

	var result spec.Result
	if err := (*client).Call("Ocean.AppendEntries", *args, &result); err != nil {
		log.Fatal(err)
	}
	return result
}

func (f *Ocean) AppendEntries(args spec.AppendEntriesArgs, result *spec.Result) error {
	// If Entries is empty, this is a heartbeat.
	if len(args.Entries) == 0 {
		heartbeats <- timeMs()
	}
	// TODO (02/27 @ 11:27): implement
	config.LogIf("[<-HEARTBEAT]", config.C.LogHeartbeats)
	return nil
}

// func putAssignC(PID int, args *spec.PutArgs) {
// 	log.SetPrefix(log.Prefix() + "putAssignC(): ")
// 	defer log.SetPrefix(config.C.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")
// 	client := connect(PID)
// 	defer client.Close()

// 	var replicas []int
// 	if err := (*client).Call("Filesystem.PutAssign", *args, &replicas); err != nil {
// 		log.Fatal(err)
// 	}
// }

// Connect to some RPC server and return a pointer to the client
// Retry some number of times if connection fails
func connect(PID int) *rpc.Client {
	node, ok := self.MemberMap[PID]
	var client *rpc.Client
	var err error
	if !ok {
		log.Fatalf("[PID=%d] member not found.", PID)
	}
	for i := 0; i < config.C.RPCMaxRetries; i++ {
		client, err = rpc.DialHTTP("tcp", (*node).IP+":"+config.C.RPCPort)
		if err != nil {
			log.Println("put() dialing:", err)
			time.Sleep(time.Second * time.Duration(config.C.RPCRetryInterval))
		} else {
			break
		}
	}
	return client
}
