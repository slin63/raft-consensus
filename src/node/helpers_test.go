// Client and server stubs for RPCs.
package node

import (
	"testing"
)

// (1) Fail if terms don't match
func TestGetTerm(t *testing.T) {
	testCases := []string{"15,text", "0,hotdog"}
	testAnswers := []int{15, 0}
	for idx, case_ := range testCases {
		if GetTerm(&case_) != testAnswers[idx] {
			t.Fatalf("Expected %d, got %d", testAnswers[idx], GetTerm(&case_))
		}
	}
}
