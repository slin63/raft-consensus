// Code needed for running elections
package node

import (
	"fmt"
	"log"
	"time"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
	"github.com/slin63/raft-consensus/pkg/responses"
)

// TODO (03/07 @ 13:11): Need to test elections when everyone has the same election timeout timer so that we can assure that elections will still complete when they are concurrent candidates
// Initiate an election. Return true if we won the election, false if we did not
func InitiateElection() bool {
	// S5.2 On conversion to candidate
	raft.Role = spec.CANDIDATE
	raft.CurrentTerm += 1
	raft.VotedFor = self.PID
	votes := 1
	quorum := spec.GetQuorum(&self)
	raft.ResetElectTimer()

	config.LogIf(fmt.Sprintf("[ELECTION->]: Starting election [TERM=%d]", raft.CurrentTerm), config.C.LogElections)

	results := make(chan *responses.Result)
	// Send out RequestVote RPCs to all other nodes
	spec.SelfRWMutex.RLock()
	for PID := range self.MemberMap {
		if PID == self.PID {
			continue
		}

		go func(PID int) {
			client, err := connect(PID, config.C.RPCPort)
			if err != nil {
				config.LogIf(fmt.Sprintf("[CONNERROR] InitiateElection failed to connect to [PID=%d]. Aborting", PID), config.C.LogConnections)
				return
			}
			defer client.Close()
			args := &spec.RequestVoteArgs{
				raft.CurrentTerm,
				self.PID,
				raft.GetLastLogIndex(),
				raft.GetLastLogTerm(),
			}

			var result responses.Result
			if err := client.Call("Ocean.RequestVote", args, &result); err != nil {
				log.Fatal("Ocean.RequestVote failed:", err)
			}
			select {
			// Receiver channel ready to receive results
			case results <- &result:
			// Receiver no longer doing its thing. Move on!
			case <-time.After(time.Duration(config.C.RPCTimeout) * time.Second):
				config.LogIf(fmt.Sprintf("[REQUESTVOTES-X] Receiver channel blocked. Aborting"), config.C.LogElections)
			}
			return
		}(PID)
	}
	spec.SelfRWMutex.RUnlock()
	config.LogIf(fmt.Sprintf("[ELECTION->]: Starting election 2"), config.C.LogElections)

	// Process results as they come in, become the leader if we receive enough votes
	for {
		select {
		case r := <-results:
			spec.RaftRWMutex.Lock()
			config.LogIf(fmt.Sprintf("[CANDIDATE]: Processing results. %d/%d needed", votes, quorum), config.C.LogElections)
			// Secede to nodes with higher terms
			if r.Term > raft.CurrentTerm {
				config.LogIf(fmt.Sprintf("[ELECTION-X]: Found node with higher term. Stepping down and resetting election state."), config.C.LogElections)
				raft.ResetElectionState(r.Term)
				raft.ElectTimer.Stop()
				spec.RaftRWMutex.Unlock()

				return false
			}
			if r.VoteGranted {
				votes += 1
				if votes >= quorum {
					config.LogIf(fmt.Sprintf("[CANDIDATE]: QUORUM received (%d/%d)", votes, quorum), config.C.LogElections)
					raft.BecomeLeader(&self)
					spec.RaftRWMutex.Unlock()

					return true
				}
			}
			spec.RaftRWMutex.Unlock()
		case t := <-endElection:
			// The election is over. Stop our timer and reset election state to that of a follower.
			config.LogIf(fmt.Sprintf("[ENDELECTION]: End election pre-lock"), config.C.LogElections)
			spec.RaftRWMutex.Lock()
			config.LogIf(fmt.Sprintf("[ENDELECTION]: End election signal received. Resetting election state. New [TERM=%d]", t), config.C.LogElections)
			raft.ResetElectionState(t)
			raft.ElectTimer.Stop()
			spec.RaftRWMutex.Unlock()

			return false
		}
	}
}

func (f *Ocean) RequestVote(a spec.RequestVoteArgs, result *responses.Result) error {
	// Step down and update term if we receive a higher term
	if a.Term > raft.CurrentTerm {
		if raft.Role == spec.CANDIDATE {
			config.LogIf(
				fmt.Sprintf(
					"[<-ELECTIONERR]: [ME=%d] [TERM=%d] Received RequestVote with higher term [%d:%d]. Ending election!",
					self.PID, raft.CurrentTerm, a.Term, raft.CurrentTerm),
				config.C.LogElections)
			endElection <- a.Term
		} else {
			spec.RaftRWMutex.Lock()
			raft.CurrentTerm = a.Term
			raft.Role = spec.FOLLOWER
			spec.RaftRWMutex.Unlock()
		}
		// This is a new term. We can reset VotedFor.
		raft.VotedFor = spec.NOCANDIDATE
	}

	// (1) S5.1 Fail if our term is greater
	if raft.CurrentTerm > a.Term {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: MISMATCHTERM"), config.C.LogElections)
		*result = responses.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: MISMATCHTERM}
		return nil
	}

	// (2) S5.2, S5.4 Make sure we haven't already voted for someone else or for this PID
	if raft.VotedFor != spec.NOCANDIDATE {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: ALREADYVOTED [raft.VotedFor=%d]", raft.VotedFor), config.C.LogElections)
		*result = responses.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: ALREADYVOTED}
		return nil
	}

	// Make sure candidate's log is at least as up-to-date as our log by
	// (a) Comparing log terms and (b) log length
	if a.LastLogTerm < spec.GetTerm(raft.GetLastEntry()) {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: OUTDATEDLOGTERM"), config.C.LogElections)
		*result = responses.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: OUTDATEDLOGTERM}
		return nil
	} else if a.LastLogTerm == spec.GetTerm(raft.GetLastEntry()) {
		if a.LastLogIndex < len(raft.Log)-1 {
			config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: OUTDATEDLOGLENGTH"), config.C.LogElections)
			*result = responses.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: OUTDATEDLOGLENGTH}
			return nil
		}
	}

	// If we made it to this point, the incoming log is as up-to-date as ours
	// and we can safely grant our vote and reset our election timer.
	raft.ResetElectTimer()
	*result = responses.Result{Term: raft.CurrentTerm, VoteGranted: true}
	spec.RaftRWMutex.Lock()
	raft.VotedFor = a.CandidateId
	spec.RaftRWMutex.Unlock()
	config.LogIf(fmt.Sprintf("[<-ELECTION]: [ME=%d] GRANTED RequestVote for %d", self.PID, a.CandidateId), config.C.LogElections)

	return nil
}
