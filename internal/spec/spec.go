// Constants for configuration and dealing with the membership layer
package spec

import (
	"fmt"
	"log"
	"net/rpc"
	"sync"
	"time"

	"github.com/slin63/raft-consensus/internal/config"
)

// Concurrency primitives
var SelfRWMutex sync.RWMutex
var RaftRWMutex sync.RWMutex

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
	Rejoin       bool
}

func ReportOnline(to int64) {
	log.Printf("[ONLINE] [ELECTTIMEOUT=%d]", to)
}

// Query the membership service running on the same machine for membership information.
func GetSelf(self *Self) bool {
	var client *rpc.Client
	var err error
	for i := 0; i <= config.C.MemberRPCRetryMax; i++ {
		time.Sleep(time.Duration(config.C.MemberRPCRetryInterval) * time.Second)
		client, err = rpc.DialHTTP("tcp", "localhost:"+config.C.MemberRPCPort)
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
	defer SelfRWMutex.Unlock()
	*self = reply
	config.LogIf(fmt.Sprintf("[SELF]: Updated membership"), config.C.LogMembership)
	return self.Rejoin
}
