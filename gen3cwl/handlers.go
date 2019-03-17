package gen3cwl

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// WorkflowRequest ...
type WorkflowRequest struct {
	Workflow json.RawMessage
	Inputs   json.RawMessage
}

// HandleRoot registers root endpoint
func HandleRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Print(r.URL)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Print(err)
		http.Error(w, "Please provide workflow and inputs json", 400)
		return
	}
	var content WorkflowRequest
	err = json.Unmarshal(body, &content)
	if err != nil {
		fmt.Printf("fail to parse json %v", err)

		http.Error(w, ParseError(err).Error(), 400)
		return
	}
	err = RunWorkflow(content.Workflow)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
}

// HandleHealthcheck registers root endpoint
func HandleHealthcheck(w http.ResponseWriter, r *http.Request) {
	fmt.Print(r.URL)
	return
}
