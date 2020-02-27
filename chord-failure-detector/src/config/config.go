package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

type configParam struct {
	Introducer string
}

// Introducer grabs the address of our introducer node.
func Introducer() string {
	configParams, err := parseJSON(os.Getenv("CONFIG"))
	if err != nil {
		log.Fatal(err)
	}
	return configParams.Introducer
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
