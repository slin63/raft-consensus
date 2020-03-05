// Client and server stubs for RPCs.
package node

import (
	"strings"
	"testing"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

var oc *Ocean = new(Ocean)

func init() {
	config.C.LogAppendEntries = false
	heartbeats = make(chan int64, 10)

}

// See that heartbeats are working
func TestAppendEntriesHeartbeat(t *testing.T) {
	raft = getRaft()
	self = spec.Self{PID: 1}
	argsInit := raft.GetAppendEntriesArgs(&self)
	result := getResult()

	oc.AppendEntries(*argsInit, result)
	if !result.Success {
		t.Fatalf("Failed: Failed when should have succeeded")
	}
}

// See that heartbeats fail on a term mismatch
func TestAppendEntriesHeartbeat1(t *testing.T) {
	raft = getRaft()
	self = spec.Self{PID: 1}
	argsInit := raft.GetAppendEntriesArgs(&self)
	result := getResult()
	argsInit.Term = 555

	oc.AppendEntries(*argsInit, result)
	if result.Error != MISMATCHTERM || result.Success {
		t.Fatalf("Failed: (1) Fail if terms don't match")
	}
}

// Run through each of the 3 conditions described in the paper

// (1) Fail if terms don't match
func TestAppendEntriesPut1(t *testing.T) {
	argsInit := getArgs()
	raft = getRaft()
	result := getResult()
	argsInit.Term = 555

	oc.AppendEntries(*argsInit, result)
	if result.Error != MISMATCHTERM || result.Success {
		t.Fatalf("Failed: (1) Fail if terms don't match")
	}
}

// (2) Fail if previous entry doesn't exist
func TestAppendEntriesPut2A(t *testing.T) {
	argsInit := getArgs()
	raft = getRaft()
	result := getResult()
	argsInit.PrevLogIndex = 1

	oc.AppendEntries(*argsInit, result)
	if result.Error != MISSINGLOGENTRY || result.Success {
		t.Fatalf("Failed: (2) Fail if previous entry doesn't exist")
	}
}

// (2) Fail if entry for previous term is inconsistent
func TestAppendEntriesPut2B(t *testing.T) {
	argsInit := getArgs()
	raft = getRaft()
	result := getResult()
	raft.Log[0] = "3,test"

	oc.AppendEntries(*argsInit, result)
	if result.Error != MISMATCHLOGTERM || result.Success {
		t.Fatalf("Failed: (2) Fail if previous entry doesn't exist")
	}
	// TODO (03/02 @ 15:05): handle test case with PrevLogTerm also
}

// (3) Delete conflicting entries
func TestAppendEntriesPut3(t *testing.T) {
	argsInit := getArgs()
	raft = getRaft()
	result := getResult()
	raft.CurrentTerm = 1
	argsInit.Term = 1
	argsInit.LeaderCommit = 1
	argsInit.Entries = []string{"1,test2", "1,hotdog", "1,nightmare"}
	raft.Log = []string{"0,test", "0,test1"}
	expected := []string{"0,test", "1,test2", "1,hotdog", "1,nightmare"}

	oc.AppendEntries(*argsInit, result)
	if result.Error != CONFLICTINGENTRY || !result.Success {
		t.Fatalf(string(result.Error))
	}

	if strings.Join(raft.Log, "") != strings.Join(expected, "") {
		t.Fatalf("Expected %v, got %v", expected, raft.Log)
	}

	if raft.CommitIndex != argsInit.LeaderCommit {
		t.Fatalf("Expected commit index %d, got %d", argsInit.LeaderCommit, raft.CommitIndex)
	}
}

func TestRequestVote(t *testing.T) {
	raft = getRaft()
	result := getResult()
	result.Error = NONE
	oc.RequestVote(spec.RequestVoteArgs{}, result)
	assertResult(result.Error, NONE, t)
}

func TestRequestVote1(t *testing.T) {
	raft = getRaft()
	result := getResult()
	oc.RequestVote(spec.RequestVoteArgs{Term: -1}, result)
	assertResult(result.Error, MISMATCHTERM, t)
}

func TestRequestVote2(t *testing.T) {
	raft = getRaft()
	raft.VotedFor = 5
	result := getResult()
	oc.RequestVote(spec.RequestVoteArgs{CandidateId: 1}, result)
	assertResult(result.Error, ALREADYVOTED, t)

	result.Error = NONE
	oc.RequestVote(spec.RequestVoteArgs{CandidateId: 5}, result)
	assertResult(result.Error, NONE, t)
}

func TestRequestVote3a(t *testing.T) {
	raft = getRaft()
	raft.Log = []string{"0,test", "1,test", "2,test", "2,test"}
	raft.CurrentTerm = 2
	result := getResult()
	oc.RequestVote(spec.RequestVoteArgs{Term: 2, LastLogTerm: 1}, result)
	assertResult(result.Error, OUTDATEDLOGTERM, t)
	assertResult(result.Term, 2, t)
}

func TestRequestVote3b(t *testing.T) {
	raft = getRaft()
	raft.Log = []string{"0,test", "1,test", "2,test", "2,test"}
	raft.CurrentTerm = 2
	result := getResult()
	oc.RequestVote(spec.RequestVoteArgs{Term: 2, LastLogTerm: 2, LastLogIndex: 2}, result)
	assertResult(result.Error, OUTDATEDLOGLENGTH, t)
	assertResult(result.Term, 2, t)
}

func assertResult(value int, expected int, t *testing.T) {
	if value != expected {
		t.Fatalf("Expected value to be %d, got %d", expected, value)
	}
}

func getResult() *spec.Result {
	return &spec.Result{}
}

func getArgs() *spec.AppendEntriesArgs {
	return &spec.AppendEntriesArgs{
		Term:         0,
		LeaderId:     99,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []string{"0, test"},
		LeaderCommit: 0,
	}
}

func getRaft() *spec.Raft {
	return &spec.Raft{
		CurrentTerm: 0,
		Log:         []string{"0"},
		CommitIndex: 0,
		LastApplied: 0,
	}
}
