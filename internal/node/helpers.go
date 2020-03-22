// Small helper functions
package node

import (
    "errors"
    "fmt"
    "log"
    "net"
    "net/http"
    "net/rpc"
    "time"

    "github.com/slin63/raft-consensus/internal/config"
)

func timeMs() int64 {
    return time.Now().UnixNano() / int64(time.Millisecond)
}

func serveOceanRPC() {
    oc := new(Ocean)
    rpc.Register(oc)
    rpc.HandleHTTP()
    l, e := net.Listen("tcp", ":"+config.C.RPCPort)
    if e != nil {
        log.Fatal("[ERROR] serveOceanRPC():", e)
    }
    log.Println("[RPC] serveOceanRPCs")
    http.Serve(l, nil)
}

// Connect to some RPC server and return a pointer to the client
// Retry some number of times if connection fails
func connect(PID int, port string) (*rpc.Client, error) {
    var client *rpc.Client
    var err error
    node, ok := self.MemberMap[PID]
    if !ok {
        config.LogIf(fmt.Sprintf("[CONNERROR-X] Was attempting to dial dead PID. [PID=%d] [ERR=%v]", PID, err), config.C.LogConnections)
        return client, errors.New(fmt.Sprintf("Node with [PID=%d] is dead.", PID))
    }
    c := make(chan *rpc.Client)

    // Timeout if dialing takes too long. (https://github.com/golang/go/wiki/Timeouts)
    go func() {
        for i := 0; i < config.C.RPCMaxRetries; i++ {
            client, err := rpc.DialHTTP("tcp", node.IP+":"+port)
            if err != nil {
                _, ok := self.MemberMap[PID]
                if !ok {
                    config.LogIf(fmt.Sprintf("[CONNERROR-X] Was attempting to dial dead PID. [PID=%d] [ERR=%v]", PID, err), config.C.LogConnections)
                    return
                }
                config.LogIf(fmt.Sprintf("[CONNERROR->] Failed to dial [PID=%d] [ERR=%v]", PID, err), config.C.LogConnections)
                time.Sleep(time.Second * time.Duration(config.C.RPCRetryInterval))
            } else {
                c <- client
                return
            }
        }
    }()

    select {
    // Received a response. Handle error appropriately
    case client = <-c:
    // Timed out waiting for a response. Wait and try again.
    case <-time.After(time.Duration(config.C.RPCTimeout) * time.Second):
        config.LogIf(fmt.Sprintf("[CONNERROR-X] Timed out dialing %d", PID), config.C.LogConnections)
        err = errors.New(fmt.Sprintf("Timed out waiting for response."))
    }

    return client, err
}

func tr(s string, max int) string {
    if len(s) > max {
        return s[0:max] + "..."
    }
    return s
}
