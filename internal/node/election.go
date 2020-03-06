// Code needed for running elections
package node

import (
	"fmt"
	"log"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

func InitiateElection() {
	config.LogIf(fmt.Sprintf("[ELECTION->]: PRE-LOCK Starting election"), config.C.LogElections)
	spec.RaftRWMutex.Lock()
	defer spec.RaftRWMutex.Unlock()
	spec.SelfRWMutex.RLock()
	defer spec.SelfRWMutex.RUnlock()
	config.LogIf(fmt.Sprintf("[ELECTION->]: POST-LOCK Starting election"), config.C.LogElections)

	// S5.2 On conversion to candidate
	raft.Role = spec.CANDIDATE
	raft.CurrentTerm += 1
	raft.VotedFor = self.PID
	votes := 1
	quorum := spec.GetQuorum(&self)
	raft.ResetElectTimer()

	results := make(chan *spec.Result)
	// TODO (03/05 @ 18:00): terminate this with cancelElection or raft.ElectTimer
	// Send out RequestVote RPCs to all other nodes
	for PID := range self.MemberMap {
		if PID == self.PID {
			continue
		}

		go func(PID int) {
			client := connect(PID)
			defer client.Close()
			args := &spec.RequestVoteArgs{
				raft.CurrentTerm,
				self.PID,
				raft.GetLastLogIndex(),
				raft.GetLastLogTerm(),
			}

			var result spec.Result
			if err := client.Call("Ocean.RequestVote", args, &result); err != nil {
				log.Fatal("Ocean.RequestVote failed:", err)
			}
			results <- &result
		}(PID)
	}

	// Process results as they come in, become the leader if we receive enough votes
	for r := range results {
		config.LogIf(fmt.Sprintf("[CANDIDATE]: Processing results"), config.C.LogElections)
		// Secede to nodes with higher terms
		if r.Term > raft.CurrentTerm {
			raft.CurrentTerm = r.Term
			endElection <- 1
		}

		if r.VoteGranted {
			votes += 1
			if votes >= quorum {
				config.LogIf(fmt.Sprintf("[CANDIDATE]: QUORUM Received (%d/%d)", votes, quorum), config.C.LogElections)
				endElection <- 1
				raft.BecomeLeader(&self)
				break
			}
		}
	}
	config.LogIf(fmt.Sprintf("[CANDIDATE]: DONE"), config.C.LogElections)
}

// type RequestVoteArgs struct {
//     // Term and ID of candidate requesting vote
//     Term        int
//     CandidateId int

//     // Index and term of candidate's last log entry
//     LastLogIndex int
//     LastLogTerm  int
// }
