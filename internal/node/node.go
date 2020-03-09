// Stuff the server needs to do to stay alive and do its job.
package node

import (
    "fmt"
    "runtime"

    "log"
    "os"

    "sync"
    "time"

    "github.com/slin63/raft-consensus/internal/config"
    "github.com/slin63/raft-consensus/internal/spec"
)

// Raft state
var raft *spec.Raft

// Membership layer state
var self spec.Self

var heartbeats = make(chan int)
var membershipUpdate = make(chan struct{})
var endElection = make(chan struct{})
var block = make(chan int, 1)

func Live() {
    // Initialize logging to file
    f, err := os.OpenFile(config.C.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
    if err != nil {
        log.Fatalf("error opening file: %v", err)
    }
    defer f.Close()

    // Get initial membership info
    spec.GetSelf(&self)
    log.SetPrefix(config.C.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")

    // Create our raft instance
    raft = &spec.Raft{
        Log:          []string{"0,NULL"},
        ElectTimeout: spec.ElectTimeout(),
        Wg:           &sync.WaitGroup{},
    }
    raft.Init(&self)

    spec.ReportOnline(raft.ElectTimeout)
    if config.C.LogGoroutines {
        go logGoroutines()
    }
    go live()
    <-block
}

func live() {
    go serveOceanRPC()
    go subscribeMembership(membershipUpdate)
    go heartbeat()
    go dispatchHeartbeats()

    // Wait for other nodes to come online
    // Start the election timer
    time.Sleep(time.Second * 1)
    raft.ResetElectTimer()
}

// Leaders beat their hearts
// Followers listen to heartbeats and initiate elections after enough time is passed
func heartbeat() {
    for {
        if raft.Role == spec.LEADER {
            // Send empty append entries to every member as goroutines
            for PID := range self.MemberMap {
                if PID != self.PID {
                    heartbeats <- PID
                }
            }

            time.Sleep(time.Duration(config.C.HeartbeatInterval) * time.Millisecond * time.Duration(config.C.Timescale))
        } else {
            // Watch for election timer timeouts
            select {
            case <-raft.ElectTimer.C:
                if raft.Role == spec.CANDIDATE {
                    config.LogIf(fmt.Sprintf("[ELECTTIMEOUT] CANDIDATE timed out while waiting for votes"), config.C.LogElections)
                } else {
                    log.Println("[ELECTTIMEOUT]")
                }
                go InitiateElection()

            default:
                continue
            }
        }
    }
}

// Check safety of heartbeat target and dispatch heartbeat goroutine
func dispatchHeartbeats() {
    for PID := range heartbeats {
        args := raft.GetAppendEntriesArgs(&self)
        // log.Println("dispatchHeartbeats() Goroutine count:", runtime.NumGoroutine())
        if _, ok := self.MemberMap[PID]; !ok {
            config.LogIf(
                fmt.Sprintf("[HEARTBEATERR] Tried heartbeating to dead node [PID=%d].", PID),
                config.C.LogHeartbeats,
            )
            return
        } else {
            go func(PID int) {
                config.LogIf(
                    fmt.Sprintf("[LEAD] [HEARTBEAT->]: to [PID=%d]", PID),
                    config.C.LogHeartbeatsLead,
                )
                r := CallAppendEntries(PID, args)
                config.LogIf(
                    fmt.Sprintf("[LEAD] [HEARTBEAT->]: DONE FROM [PID=%d]", PID),
                    (config.C.LogHeartbeatsLead && r.Error != CONNERROR),
                )
            }(PID)
        }
    }

}

// Periodically poll for membership information
func subscribeMembership(membershipUpdate chan<- struct{}) {
    for {
        spec.GetSelf(&self)
        time.Sleep(time.Second * spec.MemberInterval)
    }
}

func logGoroutines() {
    for {
        log.Printf("[GOROUTINES]: %d", runtime.NumGoroutine())
        time.Sleep(time.Second * 5)
    }
}
