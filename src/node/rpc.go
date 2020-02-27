// Client and server stubs for RPCs.
package node

import (
	"log"
	"net"
	"net/http"
	"net/rpc"

	"../config"
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
	http.Serve(l, nil)
}

// // Put (from: client)
// // Hash the file onto some appropriate point on the ring.
// // Message that point on the ring with the filename and data.
// // Respond to the client with the process ID of the server that was selected.
// func (f *Ocean) Put(args spec.PutArgs, PIDPtr *int) error {
// 	log.SetPrefix(log.Prefix() + "Put(): ")
// 	defer log.SetPrefix(spec.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")
// 	if self.M != 0 {
// 		FPID := hashing.MHash(args.Filename, self.M)
// 		PIDPtr = spec.GetSuccPID(FPID, &self)

// 		// Dispatch PutAssign RPC or perform on self
// 		if *PIDPtr != self.PID {
// 			args.From = self.PID
// 			putAssignC(*PIDPtr, &args)
// 		} else {
// 			_putAssign(&args)
// 		}
// 	}
// 	return nil
// }

// Connect to some RPC server and return a pointer to the client
func connect(PID int) *rpc.Client {
	node, ok := self.MemberMap[PID]
	if !ok {
		log.Fatalf("[PID=%d] member not found.", PID)
	}
	client, err := rpc.DialHTTP("tcp", (*node).IP+":"+config.C.RPCPort)
	if err != nil {
		log.Fatal("put() dialing:", err)
	}

	return client
}
