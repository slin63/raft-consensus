// Code for communicating with the client
package node

import (
    "fmt"
    "log"
    "time"

    "github.com/slin63/raft-consensus/internal/config"
    "github.com/slin63/raft-consensus/internal/spec"
    "github.com/slin63/raft-consensus/pkg/responses"
)

// Both the following structs contain a channel that
// digestEntries will use to notify the upstream
// PutEntry of successful replication/commit application
type entryC struct {
    // Entry to be appended to log
    D string
    C chan *responses.Result
}
type commitC struct {
    // An index to be committed
    Idx int
    C   chan *responses.Result
}

// Receive entries from a client to be added to our log
//  - It is up to our downstream client to to determine whether
//    or not it's a valid entry.
//  - The leader appends the command to its log as a new entry,
//    then issues AppendEntries RPCs in parallel to each of the
//    other servers to replicate the entry.
//  - [digestEntries] processes entries chan and on successful replication,
//    sends a success signal back upstream to PutEntry to indicate the entry
//    was safely replicated
//  - [digestCommits] processes commits chan and on successful commit, returns
//    information about the committed index back upstream to PutEntry
func (f *Ocean) PutEntry(entry string, result *responses.Result) error {
    // PutEntry can be called by the client while it is searching
    // for the leader. If so, respond with leader information
    if raft.Role != spec.LEADER {
        log.Printf("[PUTENTRY]: REDIRECTING client to leader at %d:%s", raft.LeaderId, self.MemberMap[raft.LeaderId].IP)
        *result = responses.Result{
            Data:    fmt.Sprintf("%d,%s", raft.LeaderId, self.MemberMap[raft.LeaderId].IP),
            Success: false,
            Error:   responses.LEADERREDIRECT,
        }
        return nil
    }
    log.Printf("[PUTENTRY]: BEGINNING PutEntry() FOR: %s", tr(entry, 20))

    entryCh := make(chan *responses.Result)
    commCh := make(chan *responses.Result)

    // Add new entry to log for processing
    entries <- entryC{entry, entryCh}

    select {
    case r := <-entryCh:
        r.Entry = entry
        if r.Success {
            // The entry was successfully processed.
            // Now apply to our own state.
            //   - The program will explode if the state application fails.
            commits <- commitC{r.Index, commCh}
        }
        *result = *<-commCh
    case <-time.After(time.Second * time.Duration(config.C.RPCTimeout)):
        config.LogIf(fmt.Sprintf("[PUTENTRY]: PutEntry timed out waiting for quorum"), config.C.LogPutEntry)
        *result = responses.Result{Term: raft.CurrentTerm, Success: false}
    }

    return nil
}

// As titled. Assume the following:
// 1) For any server capable of responding, we will EVENTUALLY receive
//    a successful response.
func appendEntriesUntilSuccess(raft *spec.Raft, PID int) *responses.Result {
    var result *responses.Result

    // If last log index >= nextIndex for a follower,
    // send log entries starting at nextIndex.
    // (??) Otherwise set NextIndex[PID] to len(raft.Log)-1
    if len(raft.Log)-1 < raft.NextIndex[PID] {
        log.Printf("[PUTENTRY-X]: [len(raft.Log)-1=%d] [raft.NextIndex[PID]=%d]\n", len(raft.Log)-1, raft.NextIndex[PID])
        raft.NextIndex[PID] = len(raft.Log) - 1
    }

    log.Printf("[PUTENTRY->]: [PID=%d]", PID)
    for {
        // Regenerate arguments on each call, because
        // raft state may have changed between calls
        spec.RaftRWMutex.RLock()
        args := raft.GetAppendEntriesArgs(&self)
        args.PrevLogIndex = raft.NextIndex[PID] - 1
        args.PrevLogTerm = spec.GetTerm(&raft.Log[args.PrevLogIndex])
        args.Entries = raft.Log[raft.NextIndex[PID]:]
        config.LogIf(
            fmt.Sprintf("appendEntriesUntilSuccess() to [PID=%d] with args: T:%v, L:%v, PLI:%v, PLT:%v, LC:%v",
                PID,
                args.Term,
                args.LeaderId,
                args.PrevLogIndex,
                args.PrevLogTerm,
                args.LeaderCommit,
            ),
            config.C.LogAppendEntries)
        spec.RaftRWMutex.RUnlock()
        result = CallAppendEntries(PID, args)
        log.Println(result)

        // Success! Increment next/matchIndex as a function of our inputs
        // Otherwise, decrement nextIndex and try again.
        spec.RaftRWMutex.Lock()
        if result.Success {
            raft.MatchIndex[PID] = args.PrevLogIndex + len(args.Entries)
            raft.NextIndex[PID] = raft.MatchIndex[PID] + 1
            spec.RaftRWMutex.Unlock()
            return result
        }

        // Decrement NextIndex if the failure was due to log consistency.
        // If not, update our term and step down
        if result.Term > raft.CurrentTerm {
            raft.CurrentTerm = result.Term
            raft.Role = spec.FOLLOWER
        }

        if result.Error != responses.CONNERROR {
            raft.NextIndex[PID] -= 1
        }

        spec.RaftRWMutex.Unlock()
    }
}
