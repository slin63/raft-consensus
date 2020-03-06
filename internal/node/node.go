// Stuff the server needs to do to stay alive and do its job.
package node

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/slin63/raft-consensus/internal/config"
	"github.com/slin63/raft-consensus/internal/spec"
)

// Raft state
var raft *spec.Raft
var leader bool

// Membership layer state
var self spec.Self

var endElection = make(chan int, 1)
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
	go live()
	<-block
}

func live() {
	go serveOceanRPC()
	go subscribeMembership()
	go heartbeat()

	// Wait for other nodes to come online
	// Start the election timer
	time.Sleep(time.Second * 1)
	raft.ResetElectTimer()
}

// Leaders beat their hearts
// Followers listen to heartbeats and initiate elections after enough time is passed
func heartbeat() {
	for {
		if leader {
			// Send empty append entries to every member as goroutines
			spec.SelfRWMutex.RLock()
			for PID := range self.MemberMap {
				if PID != self.PID {
					go func(PID int) {
						CallAppendEntries(PID, raft.GetAppendEntriesArgs(&self))
						config.LogIf(
							fmt.Sprintf("[HEARTBEAT->]: [PID=%d]", PID),
							config.C.LogHeartbeats,
						)
					}(PID)
				}
			}
			spec.SelfRWMutex.RUnlock()

			time.Sleep(time.Duration(config.C.HeartbeatInterval) * time.Millisecond)
		} else {
			select {
			case <-raft.ElectTimer.C:
				if raft.Role == spec.CANDIDATE {
					config.LogIf(fmt.Sprintf("[ELECTTIMEOUT] CANDIDATE timed out while waiting for votes"), config.C.LogElections)
					endElection <- 1
				} else {
					log.Println("[ELECTTIMEOUT]")
					go InitiateElection()
				}

			}
		}
	}
}

// Periodically poll for membership information
func subscribeMembership() {
	for {
		spec.GetSelf(&self)
		time.Sleep(time.Second * spec.MemberInterval)
	}
}
