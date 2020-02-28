// Raft state and operations
package spec

import (
	"fmt"
	"math/rand"

	"../config"
)

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

	ElectTimeout int64
}

type Result struct {
	// currentTerm, for leader to update itself
	Term int
	// true if follower contained entry matching prevLogIndex and prevLogTerm
	Success bool
}

type AppendEntriesArgs struct {
	Term     int
	LeaderId int

	// index of log entry immediately preceding new ones
	PrevLogIndex int
	// term of prevLogIndex entry
	PrevLogTerm int

	Entries      []string
	LeaderCommit int
}

func ElectTimeout() int64 {
	return int64(rand.Intn(config.C.ElectTimeoutMax-config.C.ElectTimeoutMin) + config.C.ElectTimeoutMin)
}

func (r *Raft) AppendEntry(msg string) *[]string {
	(*r).Log = append(
		(*r).Log,
		fmt.Sprintf("%d,%s", (*r).CurrentTerm, msg),
	)
	return &(*r).Log
}
