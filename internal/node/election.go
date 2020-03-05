// Code needed for running elections
package node

import (
	"fmt"
	"log"

	"github.com/slin63/raft-consensus/internal/spec"
)

func InitiateElection() {
	log.Printf("[ELECTION->]: Starting election", self.PID)
	spec.RaftRWMutex.Lock()
	defer spec.RaftRWMutex.Unlock()
	spec.SelfRWMutex.RLock()
	defer spec.SelfRWMutex.RUnlock()

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
		go func(PID int) {
			client := connect(PID)
			defer client.Close()
			args := &spec.RequestVoteArgs{
				raft.CurrentTerm,
				self.PID,
				raft.GetLastLogIndex(),
				raft.GetLastLogTerm(),
			}
			fmt.Println(votes)

			var result *spec.Result
			if err := client.Call("Ocean.RequestVote", args, result); err != nil {
				log.Fatal(err)
			}
			results <- result
		}(PID)
	}

	// Process results as they come in
	for r := range results {
		fmt.Println(*r)
		// Secede to nodes with higher terms
		if r.Term > raft.CurrentTerm {
			raft.CurrentTerm = r.Term
			endElection <- 1
		}

		if r.VoteGranted {
			votes += 1
			fmt.Println(votes, quorum)
		}
	}
}

// type RequestVoteArgs struct {
//     // Term and ID of candidate requesting vote
//     Term        int
//     CandidateId int

//     // Index and term of candidate's last log entry
//     LastLogIndex int
//     LastLogTerm  int
// }
