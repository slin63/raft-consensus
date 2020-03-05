package config

import (
	"log"
)

func LogIf(msg string, condition bool) {
	if condition {
		log.Println(msg)
	}
}
