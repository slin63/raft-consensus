// Small helper functions
package node

import (
	"log"
	"strconv"
	"strings"
	"time"
)

func timeMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func GetTerm(entry *string) int {
	s := strings.Split(*entry, ",")
	term, err := strconv.Atoi(s[0])
	if err != nil {
		log.Printf("GetTerm(): %v", err)
	}
	return term
}
