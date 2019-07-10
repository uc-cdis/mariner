package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// WorkflowRequest ...
type WorkflowRequest struct {
	Workflow json.RawMessage `json:"workflow"`
	Input    json.RawMessage `json:"input"`
	ID       string          `json:"id"`
}

// HandleRoot registers root endpoint
func HandleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handling root mariner request..")
	fmt.Println(r.URL)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Print(err)
		http.Error(w, "Please provide workflow and inputs json", 400)
		return
	}
	var content WorkflowRequest
	err = json.Unmarshal(body, &content)
	if err != nil {
		fmt.Printf("fail to parse json %v\n", err)
		http.Error(w, ParseError(err).Error(), 400)
		return
	}
	// HERE - dispatch job to k8s with mariner container to run workflow
	fmt.Printf("running workflow for user %v\n", content.ID)
	err = DispatchWorkflowJob(content)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	/*
		fmt.Println("workflow:")
		PrintJSON(content.Workflow)
		fmt.Println("input:")
		PrintJSON(content.Input)
		fmt.Println("id:")
		PrintJSON(content.ID)

		engine := new(K8sEngine)
		engine.Commands = make(map[string][]string)
		engine.FinishedProcs = make(map[string]*Process)
		engine.UnfinishedProcs = make(map[string]*Process)
		err = RunWorkflow(content.ID, content.Workflow, content.Input, engine)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	*/
}

// HandleHealthcheck registers root endpoint
func HandleHealthcheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL)
	return
}
