// Code needed for running elections
package node

import (
	"fmt"
	"log"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

// TODO (03/07 @ 13:11): Need to test elections when everyone has the same election timeout timer so that we can assure that elections will still complete when they are concurrent candidates
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
			log.Printf("InitiateElection() trying to connect to PID %d", PID)
			client, err := connect(PID)
			if err != nil {
				log.Printf("[CONNERROR] InitiateElection failed to connect to [PID=%d]. Aborting", PID)
				return
			}
			defer client.Close()
			args := &spec.RequestVoteArgs{
				raft.CurrentTerm,
				self.PID,
				raft.GetLastLogIndex(),
				raft.GetLastLogTerm(),
			}

			var result spec.Result
			fmt.Println("over here")
			if err := client.Call("Ocean.RequestVote", args, &result); err != nil {
				log.Fatal("Ocean.RequestVote failed:", err)
			}
			fmt.Println("over here 2 electric boogaloo")

			results <- &result
		}(PID)
	}

	// Process results as they come in, become the leader if we receive enough votes
	for {
		select {
		case r := <-results:
			config.LogIf(fmt.Sprintf("[CANDIDATE]: Processing results"), config.C.LogElections)
			// Secede to nodes with higher terms
			if r.Term > raft.CurrentTerm {
				raft.CurrentTerm = r.Term
				close(endElection)
			}

			if r.VoteGranted {
				votes += 1
				if votes >= quorum {
					config.LogIf(fmt.Sprintf("[CANDIDATE]: QUORUM Received (%d/%d)", votes, quorum), config.C.LogElections)
					close(endElection)
					raft.BecomeLeader(&self)
					return
				}
			}
		case <-endElection:
			config.LogIf(fmt.Sprintf("[ELECTION-X]: End election signal received"), config.C.LogElections)
			return
		}

	}
}
