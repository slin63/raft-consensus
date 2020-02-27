// Actual code implementations of RPCs
package node

// import (
// 	"fmt"
// 	"log"

// 	"../filesys"
// 	"../spec"
// )

// // Store the filename and data on this machine
// // Also dispatch RPC calls to replica nodes
// func _putAssign(args *spec.PutArgs) {
// 	log.SetPrefix(log.Prefix() + "_putAssign(): ")
// 	defer log.SetPrefix(spec.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")
// 	v, ok := store[(*args).Filename]
// 	if ok {
// 		log.Printf("Updating %s:%d -> %d", (*args).Filename, v, len((*args).Data))
// 	} else {
// 		n.88llog.Printf("Setting %s:%d", (*args).Filename, len((*args).Data)) 10:54 AM

// 		Hey did you see my post about going to UCB thurs night?

// 		https://hellskitchen.ucbtheatre.com/performance/72578e3se8
// 	}
// 	// Update in memory store
// 	store[(*args).Filename] = len((*args).Data)

// 	// Actually write to filesystem
// 	filesys.Write((*args).Filename, (*args).Data)
// }
