// Client and server stubs for RPCs.
package node

import (
	"fmt"
	"log"
	"math"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

// Because rafts float in the ocean
type Ocean int

// RPC Error Enums
const (
	NONE = iota
	MISMATCHTERM
	MISMATCHLOGTERM
	MISSINGLOGENTRY
	CONFLICTINGENTRY
	ALREADYVOTED
	OUTDATEDLOGTERM
	OUTDATEDLOGLENGTH
	CONNERROR
)

// AppendEntries (client)
// Invoked by leader to replicate log entries (§5.3); also used as heartbeat (§5.2).
func CallAppendEntries(PID int, args *spec.AppendEntriesArgs) *spec.Result {
	config.LogIf(fmt.Sprintf("CallAppendEntries() trying to connect to PID %d", PID), config.C.LogConnections)
	client, err := connect(PID)
	if err != nil {
		config.LogIf(fmt.Sprintf("[CONNERROR] CallAppendEntries failed to connect to [PID=%d]. Aborting", PID), config.C.LogConnections)
		return &spec.Result{Success: false, Error: CONNERROR}
	}
	defer client.Close()

	var result spec.Result
	if err := (*client).Call("Ocean.AppendEntries", *args, &result); err != nil {
		log.Fatal(err)
	}
	return &result
}

func (f *Ocean) AppendEntries(a spec.AppendEntriesArgs, result *spec.Result) error {
	spec.RaftRWMutex.Lock()
	// log.Println("AppendEntries( )Goroutine count:", runtime.NumGoroutine())
	defer spec.RaftRWMutex.Unlock()

	// (0) If their term is greater, update our term and convert to follower
	if a.Term >= raft.CurrentTerm {
		raft.CurrentTerm = a.Term
		if raft.Role == spec.CANDIDATE {
			config.LogIf(
				fmt.Sprintf("[<-APPENDENTRIES]: [ME=%d] [TERM=%d] Received AppendEntries while role=candidate. Ending election!",
					self.PID, raft.CurrentTerm),
				config.C.LogElections)
			endElection <- struct{}{}
		}
		raft.Role = spec.FOLLOWER
	}

	// (1) Fail if our term is greater
	if a.Term < raft.CurrentTerm {
		*result = spec.Result{
			Term:    raft.CurrentTerm,
			Success: false,
			Error:   MISMATCHTERM,
		}
		config.LogIf(
			fmt.Sprintf("[APPENDENTRIES] (1) Our term is greater. [(us) %d > (them) %d]", raft.CurrentTerm, a.Term),
			config.C.LogAppendEntries,
		)
		return nil
	}

	// (2) Fail if previous entry doesn't exist
	if a.PrevLogIndex >= len(raft.Log) {
		config.LogIf(
			fmt.Sprintf("[APPENDENTRIES] (2) raft.Log[PrevLogIndex=%d] does not exist. [raft.Log=%v]", a.PrevLogIndex, raft.Log),
			config.C.LogAppendEntries,
		)
		*result = spec.Result{
			Term:    raft.CurrentTerm,
			Success: false,
			Error:   MISSINGLOGENTRY,
		}
		return nil
	}

	// (2) Fail if entry for previous term is inconsistent
	if spec.GetTerm(&raft.Log[a.PrevLogIndex]) != a.PrevLogTerm {
		config.LogIf(
			fmt.Sprintf(
				"[APPENDENTRIES] (2) Log terms at index %d didn't match [(us) %d != (them) %d]",
				a.PrevLogIndex,
				spec.GetTerm(&raft.Log[a.PrevLogIndex]),
				a.PrevLogTerm,
			),
			config.C.LogAppendEntries,
		)
		*result = spec.Result{
			Term:    raft.CurrentTerm,
			Success: false,
			Error:   MISMATCHLOGTERM,
		}
		return nil
	}

	*result = spec.Result{Term: raft.CurrentTerm, Success: false}

	// (3) Delete conflicting entries
	// Check if we have conflicting entries
	if len(raft.Log) >= a.PrevLogIndex {
		newIdx := 0
		var inconsistency int
		for i := a.PrevLogIndex + 1; i < len(raft.Log); i++ {
			if spec.GetTerm(&raft.Log[i]) != spec.GetTerm(&a.Entries[newIdx]) {
				inconsistency = i
				break
			}
		}
		// Trim our logs up to the index of the term inconsistency
		if inconsistency != 0 {
			config.LogIf(
				fmt.Sprintf("[APPENDENTRIES] (3) Inconsistency in our logs, trimming to %d from original len %d", inconsistency, len(raft.Log)),
				config.C.LogAppendEntries,
			)
			raft.Log = raft.Log[:inconsistency]
		}
		result.Error = CONFLICTINGENTRY
	}

	// (4) Append any new entries not in log
	raft.Log = append(raft.Log, a.Entries...)

	// (5) Update commit index
	if a.LeaderCommit > raft.CommitIndex {
		raft.CommitIndex = int(math.Min(float64(a.LeaderCommit), float64(len(raft.Log)-1)))
		config.LogIf(
			fmt.Sprintf("[APPENDENTRIES] (5) New commit index = %d", raft.CommitIndex),
			config.C.LogAppendEntries,
		)
	}

	result.Success = true

	// If Entries is empty, this is a heartbeat.
	if len(a.Entries) == 0 {
		config.LogIf("[<-HEARTBEAT]", config.C.LogHeartbeats)
		raft.ResetElectTimer()
	} else {
		log.Printf("[<-APPENDENTRIES]: [PID=%d] [RESULT=%v] [LOGS=%v]", self.PID, *result, raft.Log)
	}

	return nil
}

// Receive entries from a client to be added to our log
// It is up to our downstream client to to determine whether
// or not it's a valid entry
// The leader appends the command to its log as a new entry,
// then issues AppendEntries RPCs in parallel to each of the
// other servers to replicate the entry.
func (f *Ocean) PutEntry(entry string, result *spec.Result) error {
	log.Printf("PutEntry(): %s", entry)

	// Add new entry to own log
	spec.RaftRWMutex.Lock()
	raft.AppendEntry(entry)
	spec.RaftRWMutex.Unlock()

	// Dispatch AppendEntries to follower nodes
	spec.SelfRWMutex.RLock()
	for PID := range self.MemberMap {
		if PID != self.PID {
			go appendEntriesUntilSuccess(raft, PID)
		}
	}
	spec.SelfRWMutex.RUnlock()
	// TODO (03/03 @ 11:07): set up quorum tracking
	*result = spec.Result{Term: raft.CurrentTerm, Success: true}
	return nil
}

func appendEntriesUntilSuccess(raft *spec.Raft, PID int) {
	var result *spec.Result
	// If last log index >= nextIndex for a follower,
	// send log entries starting at nextIndex
	if len(raft.Log)-1 >= raft.NextIndex[PID] {
		for {
			// Regenerate arguments on each call, because
			// raft state may have changed between calls
			spec.RaftRWMutex.RLock()
			args := raft.GetAppendEntriesArgs(&self)
			args.PrevLogIndex = raft.NextIndex[PID] - 1
			args.PrevLogTerm = spec.GetTerm(&raft.Log[args.PrevLogIndex])
			args.Entries = raft.Log[raft.NextIndex[PID]:]
			config.LogIf(fmt.Sprintf("appendEntriesUntilSuccess() with args: %v", args), config.C.LogAppendEntries)
			spec.RaftRWMutex.RUnlock()
			result = CallAppendEntries(PID, args)

			// Success! Increment next/matchIndex as a function of our inputs
			// Otherwise, decrement nextIndex and try again.
			spec.RaftRWMutex.Lock()
			if result.Success {
				raft.MatchIndex[PID] = args.PrevLogIndex + len(args.Entries)
				raft.NextIndex[PID] = raft.MatchIndex[PID] + 1
				break
			} else {
				// Decrement NextIndex if the failure was due to log consistency.
				// If not, update our term and step down
				if result.Term > raft.CurrentTerm {
					raft.CurrentTerm = result.Term
					raft.Role = spec.FOLLOWER
				} else {
					raft.NextIndex[PID] -= 1
				}
			}
			spec.RaftRWMutex.Unlock()
		}
	}
	log.Printf("[PUTENTRY->]: [PID=%d]", PID)
}
