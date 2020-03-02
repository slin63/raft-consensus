// Raft state and operations
package spec

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"

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

	// pid:index, for each server, index of the next log entry to send to
	// that server (initialized to leader last log index + 1)
	NextIndex map[int]int

	// pid:index, for each server, index of highest log entry known to be
	// replicated on server (initialized to 0, increases monotonically)
	MatchIndex map[int]int

	ElectTimeout int64

	Wg *sync.WaitGroup
}

type Result struct {
	// currentTerm, for leader to update itself
	Term int
	// true if follower contained entry matching prevLogIndex and prevLogTerm
	Success bool
	// error code for testing
	Error int
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

func (r *Raft) Init(self *Self) {
	RaftRWMutex.Lock()
	defer RaftRWMutex.Unlock()
	SelfRWMutex.RLock()
	defer SelfRWMutex.RUnlock()
	r.NextIndex = make(map[int]int)
	r.MatchIndex = make(map[int]int)
	for PID := range self.MemberMap {
		r.NextIndex[PID] = r.CommitIndex + 1
		r.MatchIndex[PID] = 0
	}
}

func (r *Raft) AppendEntry(msg string) (int, int, *[]string) {
	// Index of log entry immediately preceding new ones
	prevLogIndex := len((*r).Log) - 1
	prevLogTerm, err := strconv.Atoi(string((*r).Log[prevLogIndex][0]))
	if err != nil {
		log.Println("[ERROR] AppendEntry(): ", err)
	}
	entries := &([]string{msg})
	(*r).Log = append((*r).Log, fmt.Sprintf("%d,%s", (*r).CurrentTerm, msg))
	return prevLogIndex, prevLogTerm, entries
}

func (r *Raft) GetAppendEntriesArgs(self *Self) *AppendEntriesArgs {
	return &AppendEntriesArgs{
		Term:         r.CurrentTerm,
		PrevLogIndex: len(r.Log) - 1,
		PrevLogTerm:  GetTerm(&r.Log[len(r.Log)-1]),
		LeaderId:     self.PID,
		LeaderCommit: r.CommitIndex,
	}
}

func GetTerm(entry *string) int {
	s := strings.Split(*entry, ",")
	term, err := strconv.Atoi(s[0])
	if err != nil {
		log.Printf("GetTerm(): %v", err)
	}
	return term
}
