package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type configParam struct {
	// Protocol
	Introducer        string
	ElectTimeoutMin   int
	ElectTimeoutMax   int
	HeartbeatInterval int
	Timescale         int
	Quorum            float64

	// Logging
	Prefix            string
	Logfile           string
	LogHeartbeats     bool
	LogHeartbeatsLead bool
	LogAppendEntries  bool
	LogTimers         bool
	LogElections      bool
	LogMembership     bool
	LogGoroutines     bool
	LogConnections    bool

	// Networking
	RPCPort          string
	RPCMaxRetries    int
	RPCTimeout       int
	RPCRetryInterval int

	// Membership layer
	MemberRPCPort          string
	MemberRPCRetryInterval int
	MemberRPCRetryMax      int
	MemberInterval         int
}

var C configParam

func init() {
	var err error
	C, err = parseJSON(os.Getenv("CONFIG"))
	if err != nil {
		log.Fatal("Configuration error:", err)
	}
}

func parseJSON(fileName string) (configParam, error) {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		return configParam{}, err
	}

	// Necessities for go to be able to read JSON
	fileString := string(file)

	fileReader := strings.NewReader(fileString)

	decoder := json.NewDecoder(fileReader)

	var configParams configParam

	// Finally decode into json object
	err = decoder.Decode(&configParams)
	if err != nil {
		return configParam{}, err
	}

	return configParams, nil
}
