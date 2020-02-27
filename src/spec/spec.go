// Constants for configuration and dealing with the membership layer
package spec

import (
	"log"
	"net/rpc"
	"sync"
	"time"
)

// Concurrency primitives
var SelfRWMutex sync.RWMutex

// Membership RPCs
type MemberNode struct {
	// Address info formatted ip_address
	IP        string
	Timestamp int64
	Alive     bool
}
type Membership int
type SuspicionMapT map[int]int64
type FingerTableT map[int]int
type MemberMapT map[int]*MemberNode

const NILPID = -1

type Self struct {
	M            int
	PID          int
	MemberMap    MemberMapT
	FingerTable  FingerTableT
	SuspicionMap SuspicionMapT
}

// Membership layer RPC information
const MemberRPCPort = "6002"
const MemberRPCRetryInterval = 3
const MemberRPCRetryMax = 5
const MemberInterval = 5

func ReportOnline() {
	log.Printf("[ONLINE]")
}

// Query the membership service running on the same machine for membership information.
func GetSelf(self *Self) {
	var client *rpc.Client
	var err error
	for i := 0; i <= MemberRPCRetryMax; i++ {
		time.Sleep(MemberRPCRetryInterval * time.Second)
		client, err = rpc.DialHTTP("tcp", "localhost:"+MemberRPCPort)
		if err != nil {
			log.Println("RPC server still spooling... dialing:", err)
		} else {
			break
		}
	}
	// Synchronous call
	var reply Self
	err = client.Call("Membership.Self", 0, &reply)
	if err != nil {
		log.Fatal("RPC error:", err)
	}
	SelfRWMutex.Lock()
	*self = reply
	SelfRWMutex.Unlock()
}
