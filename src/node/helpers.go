// Small helper functions
package node

import "time"

func timeMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
