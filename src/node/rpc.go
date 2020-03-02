// Client and server stubs for RPCs.
package node

import (
	"fmt"
	"log"
	"math"
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

// TODO (03/02 @ 10:18): write tests for this
func (f *Ocean) AppendEntries(args spec.AppendEntriesArgs, result *spec.Result) error {
	// If Entries is empty, this is a heartbeat.
	if len(args.Entries) == 0 {
		heartbeats <- timeMs()
		config.LogIf("[<-HEARTBEAT]", config.C.LogHeartbeats)

		return nil
	}
	spec.RaftRWMutex.Lock()
	defer spec.RaftRWMutex.Unlock()

	// (1) Fail if terms don't match
	if args.Term > raft.CurrentTerm {
		*result = spec.Result{raft.CurrentTerm, false}
		config.LogIf(
			fmt.Sprintf("[PUTENTRY] (1) Terms didn't match (us) %d != (them) %d", args.Term, raft.CurrentTerm),
			config.C.LogAppendEntries,
		)
		return nil
	}

	// (2) Fail if entry for previous term is inconsistent OR doesn't exist
	if args.PrevLogIndex > len(raft.Log)-1 || string(raft.Log[args.PrevLogIndex][0]) != string(args.PrevLogTerm) {
		*result = spec.Result{raft.CurrentTerm, false}
		config.LogIf(
			fmt.Sprintf("[PUTENTRY] (2) Log terms didn't match"),
			config.C.LogAppendEntries,
		)
		return nil
	}

	// (3) Delete conflicting entries
	// Check if we have conflicting entries
	if len(raft.Log) >= args.PrevLogIndex {
		newIdx := 0
		var inconsistency int
		for i := args.PrevLogIndex + 1; i < len(raft.Log); i++ {
			if string(raft.Log[i][0]) != string(args.Entries[newIdx][0]) {
				inconsistency = i
				break
			}
		}
		// Trim our logs up to the index of the term inconsistency
		if inconsistency != 0 {
			config.LogIf(
				fmt.Sprintf("[PUTENTRY] (3) Inconsistency in our logs, trimming to %d from original len %d", inconsistency, len(raft.Log)),
				config.C.LogAppendEntries,
			)
			raft.Log = raft.Log[:inconsistency]
		}
	}

	// (4) Append any new entries not in log
	raft.Log = append(raft.Log, args.Entries...)

	// (5) Update commit index
	if args.LeaderCommit > raft.CommitIndex {
		raft.CommitIndex = int(math.Min(float64(args.LeaderCommit), float64(len(raft.Log)-1)))
		config.LogIf(
			fmt.Sprintf("[PUTENTRY] (5) New commit index = %d", raft.CommitIndex),
			config.C.LogAppendEntries,
		)
	}

	*result = spec.Result{raft.CurrentTerm, true}
	log.Printf("[<-PUTENTRY]: [PID=%d] [RESULT=%v] [LOGS=%v]", self.PID, *result, raft.Log)

	return nil
}

// Receive entries from a client to be added to our log
// It is up to our downstream client to to determine whether
// or not it's a valid entry
// The leader appends the command to its log as a new entry,
// then issues AppendEntries RPCs in parallel to each of the
// other servers to replicate the entry.
func (f *Ocean) PutEntry(entry string, result *spec.Result) error {
	spec.RaftRWMutex.Lock()
	log.Printf("PutEntry(): %s", entry)

	// Add new entry to own log
	prevLogIndex, prevLogTerm, entries := raft.AppendEntry(entry)

	// Dispatch AppendEntries to follower nodes
	spec.SelfRWMutex.RLock()
	args := &spec.AppendEntriesArgs{
		Term:         raft.CurrentTerm,
		LeaderId:     self.PID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      *entries,
		LeaderCommit: raft.CommitIndex,
	}
	for PID := range self.MemberMap {
		if PID != self.PID {
			raft.Wg.Add(1)
			go CallAppendEntries(PID, args, raft.Wg)
			log.Printf("[PUTENTRY->]: [PID=%d]", PID)
		}
	}
	raft.Wg.Wait()
	spec.SelfRWMutex.RUnlock()
	spec.RaftRWMutex.Unlock()
	*result = spec.Result{raft.CurrentTerm, true}
	return nil
}

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
		client, err = rpc.DialHTTP("tcp", node.IP+":"+config.C.RPCPort)
		if err != nil {
			log.Println("put() dialing:", err)
			time.Sleep(time.Second * time.Duration(config.C.RPCRetryInterval))
		} else {
			break
		}
	}
	return client
}
