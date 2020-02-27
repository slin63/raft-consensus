// Constants for configuration and dealing with the membership layer
package spec

import (
	"log"
	"net/rpc"
	"sync"
	"time"
)

// Semaphores
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

type Raft struct {
	// Latest term server has seen (initialized to 0 on first boot, increases monotonically)
	CurrentTerm int

	// candidateId that received vote in current term
	VotedFor int

	// log entries; each entry contains command for state machine,
	// and term when entry was received by leader (first index is 1)
	Log []string

	// index of highest log entry known to be committed
	CommitIndex int

	// index of highest log entry applied to state machine
	LastApplied int

	// for each server, index of the next log entry to send to
	// that server (initialized to leader last log index + 1)
	NextIndex int

	// for each server, index of highest log entry known to be
	// replicated on server (initialized to 0, increases monotonically)
	MatchIndex int
}

// // DFS RPCs
// type PutArgs struct {
// 	// File data
// 	Filename string
// 	Data     []byte

// 	// Server data
// 	From int
// }

// Membership layer RPC information
const MemberRPCPort = "6002"
const MemberRPCRetryInterval = 3
const MemberRPCRetryMax = 5
const MemberInterval = 5

func ReportOnline() {
	log.Printf("[ONLINE]")
}

// // Find the nearest PID to the given FPID on the virtual ring
// // (including this node's own PID)
// func GetSuccPID(FPID int, self *Self) *int {
// 	SelfRWMutex.RLock()
// 	PIDs := []int{}
// 	for PID := range (*self).MemberMap {
// 		PIDs = append(PIDs, PID)
// 	}
// 	SelfRWMutex.RUnlock()
// 	sort.Ints(PIDs)
// 	diff := 10000
// 	var succPID int
// 	FPID = FPID % (1 << self.M)

// 	log.Println("GetSuccPID(): ", PIDs)

// 	// Find the smallest (FPID - PID) that is (> 0)
// 	// in an ordered array of ints
// 	for i := 0; i < len(PIDs); i++ {
// 		iterdiff := PIDs[i] - FPID
// 		if (iterdiff) < diff && iterdiff > 0 {
// 			diff = iterdiff
// 			succPID = PIDs[i]
// 		}
// 	}
// 	return &succPID
// }

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
