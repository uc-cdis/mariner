package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// this file contains handlers for the mariner api
// right now the api is incredibly basic and needs to be extended
// pretty much all you can do at this point is run a workflow via POST to /run endpoint
// Q: are we going to implement the WES api? how important is that?
// regardless, the api needs to be extended

// WorkflowRequest holds the unmarshalled body of the POST request
// where
// Workflow is the packed CWL workflow JSON (i.e., all the CWL files packed into a JSON - ii.ee., the result of cwltool --pack)
// Input is the JSON specifying values for the input parameters to the workflow (refer to files using GUIDs)
// ID is the userID
// HERE - TODO - eventually replace "ID" field with "token"
// ---> then need to retrieve user ID by trade token with Fence
type WorkflowRequest struct {
	Workflow json.RawMessage `json:"workflow"`
	Input    json.RawMessage `json:"input"`
	ID       string          `json:"id"`    // after token flow implemented, remove this field
	Token    string          `json:"token"` // flow not yet implemented, but field defined here
	Manifest Manifest        `json:"manifest"`
}

// HERE - TODO - move to config.go
type Manifest []ManifestEntry
type ManifestEntry struct {
	GUID string `json:"object_id"`
}

// RunHandler handles `/run` endpoint
// handles a POST request to run a workflow by dispatching the workflow job
// see "../testdata/request_body.json" for an example of a valid request body
// also see above description of the fields of the WorkflowRequest struct
// since those are the same fields as the request body
// NOTE: come up with uniform, sensible names for handler functions
func RunHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handling root mariner request..")
	fmt.Println(r.URL)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Print(err)
		http.Error(w, "failed to read in workflow request", 400)
		return
	}
	var workflowRequest WorkflowRequest
	err = json.Unmarshal(body, &workflowRequest)
	if err != nil {
		fmt.Printf("fail to parse json %v\n", err)
		http.Error(w, err.Error(), 400)
		return
	}
	// dispatch job to k8s to run workflow
	// -> `mariner run $S3PREFIX`, where
	// S3PREFIX is the working directory for this workflow in the workflow bucket
	fmt.Printf("running workflow for user %v\n", workflowRequest.ID)
	PrintJSON(workflowRequest)
	err = DispatchWorkflowJob(&workflowRequest)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
}

// HandleHealthcheck registers root endpoint
func HandleHealthcheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL)
	return
}
