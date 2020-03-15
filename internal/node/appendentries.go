// Code for implementing AppendEntries
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
	OUTDATEDRESPONSE
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

	// Only process the response if we are still the same term as when we originally sent it
	if args.Term != raft.CurrentTerm {
		return &spec.Result{Success: false, Error: OUTDATEDRESPONSE}
	}

	// Our term is lower. Demote ourselves and convert to follower.
	if result.Error == MISMATCHTERM {
		spec.RaftRWMutex.Lock()
		config.LogIf(fmt.Sprintf("[MISMATCHTERM] Sent appendEntries to machine with higher Term [PID=%d]. Stepping down as leader.", PID), config.C.LogElections)
		raft.ResetElectionState(result.Term)
		raft.ElectTimer.Stop()
		spec.RaftRWMutex.Unlock()
	}

	return &result
}

func (f *Ocean) AppendEntries(a spec.AppendEntriesArgs, result *spec.Result) error {
	// (0) If their term is greater, update our term and convert to follower
	if a.Term >= raft.CurrentTerm {
		if raft.Role == spec.CANDIDATE {
			config.LogIf(
				fmt.Sprintf(
					"[<-APPENDENTRIES]: [ME=%d] [TERM=%d] Received equal or greater term AppendEntries while role=candidate. Ending election!",
					self.PID, raft.CurrentTerm,
				),
				config.C.LogElections,
			)
			endElection <- a.Term
		} else {
			spec.RaftRWMutex.Lock()
			raft.ResetElectionState(a.Term)
			spec.RaftRWMutex.Unlock()
		}
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
	spec.RaftRWMutex.Lock()
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
	spec.RaftRWMutex.Unlock()

	result.Success = true
	// If Entries is empty, this is a heartbeat.
	if len(a.Entries) == 0 {
		config.LogIf("[<-HEARTBEAT]", config.C.LogHeartbeats)
	} else {
		log.Printf("[<-APPENDENTRIES]: [PID=%d] [RESULT=%v] [LOGS=%v]", self.PID, *result, raft.Log)
	}
	raft.ResetElectTimer()

	return nil
}
