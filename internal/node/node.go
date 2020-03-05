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

var block = make(chan int, 1)

func Live(isLeader bool) {
	// If you're the leader, sleep for a little bit and let everyone else get started.
	if isLeader {
		// Sleep for a little bit so the other nodes have time to start up
		time.Sleep(time.Second * 2)
	}

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
	leader = isLeader
	raft = &spec.Raft{
		Log:          []string{"0,NULL"},
		ElectTimeout: spec.ElectTimeout(),
		Wg:           &sync.WaitGroup{},
	}
	raft.Init(&self)
	if leader {
		raft.Role = spec.LEADER
	}

	spec.ReportOnline()
	go live()

	<-block
}

func live() {
	go serveOceanRPC()
	go subscribeMembership()
	go heartbeat()
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
				log.Fatalf("[ELECTTIMEOUT]")
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
