// Code needed for running elections
package node

import (
	"fmt"
	"log"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

// TODO (03/07 @ 13:11): Need to test elections when everyone has the same election timeout timer so that we can assure that elections will still complete when they are concurrent candidates
// Initiate an election. Return true if we won the election, false if we did not
func InitiateElection() bool {
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
	// Send out RequestVote RPCs to all other nodes
	for PID := range self.MemberMap {
		if PID == self.PID {
			continue
		}

		go func(PID int) {
			// log.Printf("InitiateElection() trying to connect to PID %d", PID)
			client, err := connect(PID)
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

			var result spec.Result
			if err := client.Call("Ocean.RequestVote", args, &result); err != nil {
				log.Fatal("Ocean.RequestVote failed:", err)
			}
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
				config.LogIf(fmt.Sprintf("[ELECTION-X]: Found node with higher term. Stepping down and resetting election state."), config.C.LogElections)
				raft.ResetElectionState(raft.CurrentTerm)
				raft.ElectTimer.Stop()
				return false
			}
			if r.VoteGranted {
				votes += 1
				if votes >= quorum {
					config.LogIf(fmt.Sprintf("[CANDIDATE]: QUORUM Received (%d/%d)", votes, quorum), config.C.LogElections)
					raft.BecomeLeader(&self)
					return true
				}
			}
		case <-endElection:
			// The election is over. Stop our timer and reset election state to that of a follower.
			config.LogIf(fmt.Sprintf("[ELECTION-X]: End election signal received. Resetting election state."), config.C.LogElections)
			raft.ResetElectionState(raft.CurrentTerm)
			raft.ElectTimer.Stop()
			return false
		}

	}
}

func (f *Ocean) RequestVote(a spec.RequestVoteArgs, result *spec.Result) error {
	// config.LogIf(fmt.Sprintf("[<-ELECTION]: [ME=%d] RECEIVED RequestVote from %d", self.PID, a.CandidateId), config.C.LogElections)
	spec.RaftRWMutex.Lock()
	defer spec.RaftRWMutex.Unlock()
	// config.LogIf(fmt.Sprintf("[<-ELECTION]: [ME=%d] PROCESSING RequestVote from %d", self.PID, a.CandidateId), config.C.LogElections)

	// Step down and update term if we receive a higher term
	if a.Term > raft.CurrentTerm {
		raft.CurrentTerm = a.Term
		if raft.Role == spec.CANDIDATE {
			config.LogIf(
				fmt.Sprintf(
					"[<-ELECTIONERR]: [ME=%d] [TERM=%d] Received RequestVote with higher term [%d:%d]. Ending election!",
					a.CandidateId, a.Term, self.PID, raft.CurrentTerm),
				config.C.LogElections)
			endElection <- struct{}{}
		}
		raft.Role = spec.FOLLOWER
	}

	// (1) S5.1 Fail if our term is greater
	if raft.CurrentTerm > a.Term {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: MISMATCHTERM"), config.C.LogElections)
		*result = spec.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: MISMATCHTERM}
		return nil
	}

	// (2) S5.2, S5.4 Make sure we haven't already voted for someone else
	if raft.VotedFor != spec.NOCANDIDATE && raft.VotedFor != a.CandidateId {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: ALREADYVOTED [raft.VotedFor=%d]", raft.VotedFor), config.C.LogElections)
		*result = spec.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: ALREADYVOTED}
		return nil
	}

	// Make sure candidate's log is at least as up-to-date as our log by
	// (a) Comparing log terms and (b) log length
	if a.LastLogTerm < spec.GetTerm(raft.GetLastEntry()) {
		config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: OUTDATEDLOGTERM"), config.C.LogElections)
		*result = spec.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: OUTDATEDLOGTERM}
		return nil
	} else if a.LastLogTerm == spec.GetTerm(raft.GetLastEntry()) {
		if a.LastLogIndex < len(raft.Log)-1 {
			config.LogIf(fmt.Sprintf("[<-ELECTIONERR]: OUTDATEDLOGLENGTH"), config.C.LogElections)
			*result = spec.Result{Term: raft.CurrentTerm, VoteGranted: false, Error: OUTDATEDLOGLENGTH}
			return nil
		}
	}

	// If we made it to this point, the incoming log is as up-to-date as ours
	// and we can safely grant our vote and reset our election timer.
	raft.ResetElectTimer()
	*result = spec.Result{Term: raft.CurrentTerm, VoteGranted: true}
	config.LogIf(fmt.Sprintf("[<-ELECTION]: [ME=%d] GRANTED RequestVote for %d", self.PID, a.CandidateId), config.C.LogElections)

	return nil
}
