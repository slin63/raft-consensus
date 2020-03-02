// Client and server stubs for RPCs.
package node

import (
	"strings"
	"testing"

	"../config"
	"../spec"
)

// Run through each of the five conditions described in the paper
var oc *Ocean = new(Ocean)

func init() {
	config.C.LogAppendEntries = false
}

// (1) Fail if terms don't match
func TestAppendEntries1(t *testing.T) {
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
func TestAppendEntries2A(t *testing.T) {
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
func TestAppendEntries2B(t *testing.T) {
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
func TestAppendEntries3(t *testing.T) {
	argsInit := getArgs()
	raft = getRaft()
	result := getResult()
	raft.CurrentTerm = 1
	argsInit.Term = 1
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
