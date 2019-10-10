package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/uc-cdis/mariner/mariner"
)

/*
mariner needs to be able to:
1. setup the mariner-server to listen for API requests
2. run a workflow

usage:
 - to setup the mariner server: `mariner listen`
 - to run a workflow: `mariner run $S3PREFIX $RUN_ID`
 	 (runs workflow in /engine-workspace/workflowRuns/{runID}/request.json, which is s3://workflow-engine-garvin/userID/workflow-run-timestamp/request.json)
*/

func main() {
	switch os.Args[1] {
	case "listen":
		// mariner running in mariner-server container
		fmt.Println("setting up mariner-server..")
		mariner.Server()
	case "run":
		// mariner running in mariner-engine container
		// `bucket/userID/workflow/` has been mounted to `/data/`
		// `/data/request.json` contains the workflow, input, and id

		runID := os.Args[3] // this is a timestamp

		// NOTE: this section should be encapsulated to a function and maybe put in another file
		fmt.Println("running mariner-engine..")
		requestF, err := os.Open(fmt.Sprintf("/%v/workflowRuns/%v/request.json", mariner.ENGINE_WORKSPACE, runID))
		if err != nil {
			log.Fatal(err)
		}

		request, err := ioutil.ReadAll(requestF)
		if err != nil {
			fmt.Print(err)
			// insert better error handling here
		}
		var wfRequest mariner.WorkflowRequest
		err = json.Unmarshal(request, &wfRequest)
		if err != nil {
			fmt.Printf("fail to parse json %v\n", err)
		}
		// encapsulate this engine prep into a function - getEngine() or something like that, engine.Setup(), etc.
		// put it in mariner/engine.go
		engine := new(mariner.K8sEngine)
		engine.Commands = make(map[string][]string)
		engine.FinishedProcs = make(map[string]*mariner.Process)
		engine.UnfinishedProcs = make(map[string]*mariner.Process)

		engine.Manifest = &wfRequest.Manifest
		engine.UserID = wfRequest.ID

		// collect S3 prefix to mount from user bucket
		engine.S3Prefix = os.Args[2]
		engine.RunID = runID
		if engine.S3Prefix == "" {
			log.Fatal("missing /userID/workflow-run-timestamp/ S3 prefix")
		}

		// NOTE: the ID is not used in this processing pipeline - could remove that parameter, or keep it in case it may be needed later
		mariner.RunWorkflow(wfRequest.ID, wfRequest.Workflow, wfRequest.Input, engine)

		// tell sidecar that the workflow is done running so that container can terminate and the job can finish
		_, err = os.Create(fmt.Sprintf("/%v/workflowRuns/%v/done", mariner.ENGINE_WORKSPACE, runID))
		if err != nil {
			fmt.Println("error writing workflow-done flag")
			log.Fatal(err)
		}
	}
}
