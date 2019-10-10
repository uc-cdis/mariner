package main

import (
	"fmt"
	"log"
	"os"

	"github.com/uc-cdis/mariner/mariner"
)

/*
FIXME - revise description of executable

mariner needs to be able to:
1. setup the mariner-server to listen for API requests
2. run a workflow

usage:
 - to setup the mariner server: `mariner listen`
 - to run a workflow: `mariner run $RUN_ID`
 	 (runs workflow in /engine-workspace/workflowRuns/{runID}/request.json, which is s3://workflow-engine-garvin/userID/workflow-run-timestamp/request.json)
*/

func main() {
	switch os.Args[1] {
	case "listen":
		fmt.Println("running mariner-server..")
		mariner.Server() // should this function return an error?
	case "run":
		fmt.Println("running mariner-engine..")
		runID := os.Args[2]
		if err := mariner.Engine(runID); err != nil {
			log.Fatal(err)
		}
	}
}
