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
 - to run a workflow: `mariner run $RUN_ID`
 	 (runs workflow in /engine-workspace/workflowRuns/{runID}/request.json, which is s3://workflow-engine-garvin/userID/workflow-run-timestamp/request.json)
*/

func main() {
	switch os.Args[1] {
	case "listen":
		// mariner running in mariner-server container
		fmt.Println("running mariner-server..")
		mariner.Server()
	case "run":
		// mariner running in mariner-engine container
		fmt.Println("running mariner-engine..")

		runID := os.Args[2] // this is a timestamp

		// mariner.Engine(runID)

		// NOTE: this section should be encapsulated to a function and maybe put in another file
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
		engine.RunID = runID
		// error handling here

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
