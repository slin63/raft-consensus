package spec

import (
	"testing"

	"../config"
)

func TestElectTimeout(t *testing.T) {
	for i := 0; i < 100; i++ {
		if to := ElectTimeout(); to < int64(config.C.ElectTimeoutMin) || to > int64(config.C.ElectTimeoutMax) {
			t.Fatalf(
				"ElectTimeout() returned value outside accepted range got: %d, want: [%d,%d].",
				to,
				config.C.ElectTimeoutMin,
				config.C.ElectTimeoutMax,
			)
		}
	}
}

func TestGetQuorom(t *testing.T) {
	self := Self{MemberMap: make(MemberMapT)}
	self.MemberMap[1] = &MemberNode{}
	self.MemberMap[2] = &MemberNode{}
	self.MemberMap[3] = &MemberNode{}
	self.MemberMap[4] = &MemberNode{}
	self.MemberMap[5] = &MemberNode{}
	if GetQuorum(&self) != 3 {
		t.Fatalf("Expected 3 quorum members, got %d", GetQuorum(&self))
	}

}

func TestInit(t *testing.T) {
	raft := &Raft{
		Log: []string{"0", "1", "2"},
	}
	self := Self{MemberMap: make(MemberMapT)}
	self.MemberMap[1] = &MemberNode{}
	self.MemberMap[2] = &MemberNode{}
	raft.Init(&self)
	for _, idx := range raft.NextIndex {
		if raft.CommitIndex+1 != idx {
			t.Fatalf(
				"Expected index to be %d, but got %d", raft.CommitIndex+1, idx,
			)
		}
	}
	for _, idx := range raft.MatchIndex {
		if idx != 0 {
			t.Fatalf(
				"Expected index to be %d, but got %d", 0, idx,
			)
		}
	}
}

func TestGetTerm(t *testing.T) {
	testCases := []string{"15,text", "0,hotdog"}
	testAnswers := []int{15, 0}
	for idx, case_ := range testCases {
		if GetTerm(&case_) != testAnswers[idx] {
			t.Fatalf("Expected %d, got %d", testAnswers[idx], GetTerm(&case_))
		}
	}
}

func TestGetLastEntry(t *testing.T) {
	raft := &Raft{
		Log: []string{"0", "1", "2"},
	}

	if raft.GetLastEntry() != "2" {
		t.Fatalf(
			"Expected entry to be %s, but got %s", "2", raft.GetLastEntry(),
		)
	}
}
