package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"gopkg.in/yaml.v2"

	mariner "github.com/uc-cdis/mariner/mariner"
	wftool "github.com/uc-cdis/mariner/wftool"
)

// TestCase ..
type TestCase struct {
	Input      string                 `json:"job"`         // path to input.json (may also be yaml)
	Output     map[string]interface{} `json:"output"`      // expected output
	ShouldFail bool                   `json:"should_fail"` // if the engine is expected to fail on this cwl
	CWL        string                 `json:"tool"`        // path to tool.cwl
	Label      string                 `json:"label"`
	ID         int                    `json:"id"`
	Doc        string                 `json:"doc"`
	Tags       []string               `json:"tags"`
}

// Runner ..
type Runner struct {
	Token   string
	Results *Results
}

// Results captures test results
type Results struct {
	Pass   []int
	Fail   []int
	Manual []int // some tests need to be looked at closely, at least for now
	// guarding against false positives
}

func main() {

}

// path to config: 		./common-workflow-language/v1.0/conformance_test_v1.0.yaml
// path to test suite: 	./common-workflow-language/v1.0/v1.0/

const (
	// path to tests config - don't hardcode (?)
	config = "./common-workflow-language/v1.0/conformance_test_v1.0.yaml"
	// of course, avoid hardcoding
	// could pass commons url as param
	tokenEndpoint = "https://mattgarvin1.planx-pla.net/user/credentials/api/access_token"

	// again, don't hardcode
	runEndpoint = "https://mattgarvin1.planx-pla.net/ga4gh/wes/v1/runs"
)

// 'creds' is path/to/creds.json which is what you get
// when you create and download an apiKey from the portal
func runTests(creds string) error {
	suite, err := loadConfig(config)
	if err != nil {
		return err
	}
	tok, err := token(creds)
	if err != nil {
		return err
	}

	runner := Runner{
		Token:   tok,
		Results: new(Results),
	}

	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		// go runTest(test, tok) // todo

		// dev with sequential, then make concurrent
		if err = runner.Run(test); err != nil {
			// log/handle err
		}
	}
	return nil
}

func loadConfig(config string) ([]TestCase, error) {
	b, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	testSuite := new([]TestCase)
	if err = yaml.Unmarshal(b, testSuite); err != nil {
		return nil, err
	}
	return *testSuite, nil
}

// Creds is creds.json, as downloaded from the portal
type Creds struct {
	APIKey string `json:"api_key"`
	KeyID  string `json:"key_id"`
}

// AccessToken response from fence /access_token endpoint
type AccessToken struct {
	Token string `json:"access_token"`
}

// read creds into Creds struct
func apiKey(creds string) ([]byte, error) {
	// read in bytes
	b, err := ioutil.ReadFile(creds)
	if err != nil {
		return nil, err
	}

	// validate against creds schema
	c := &Creds{}
	err = json.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}
	if c.APIKey == "" || c.KeyID == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	// return bytes
	return b, nil
}

func token(creds string) (string, error) {
	body, err := apiKey(creds)
	if err != nil {
		return "", err
	}
	resp, err := http.Post(tokenEndpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	t := &AccessToken{}
	err = json.Unmarshal(b, t)
	if err != nil {
		return "", err
	}
	return t.Token, nil
}

/*
Short list (2/10/19):
1. when loading inputs - need to modify paths/locations/etc of file inputs
2. need to collect all file inputs so to stage in s3
3. use type from mariner for api request body
4. need a little function to match output (arbitrary map[string]interface{})
*/

// WorkflowRequest ..
type WorkflowRequest struct {
	Workflow *wftool.WorkflowJSON
	Input    map[string]interface{}
	Tags     map[string]string
}

func (t *TestCase) workflow() (*wftool.WorkflowJSON, error) {
	wf, err := wftool.PackWorkflow(t.CWL)
	if err != nil {
		return nil, err
	}
	valid, grievances := wftool.ValidateWorkflow(wf)
	if !valid {
		return nil, fmt.Errorf("%v", grievances)
	}
	return wf, nil
}

func (t *TestCase) tags() map[string]string {
	tags := make(map[string]string)
	tags["job"] = t.Input
	tags["tool"] = t.CWL
	tags["label"] = t.Label
	tags["id"] = string(t.ID)
	tags["doc"] = t.Doc
	tags["tags"] = strings.Join(t.Tags, ",")
	if t.ShouldFail {
		tags["should_fail"] = "true"
	} else {
		tags["should_fail"] = "false"
	}
	return tags
}

// todo
func (t *TestCase) input() (map[string]interface{}, error) {

	return nil, nil
}

// Run ..
// here - todo
// run the test and record test result in the runner
func (r *Runner) Run(test TestCase) error {

	fmt.Println(test)

	// make these fns methods of type TestCase

	// 1. pack the CWL to json (wf)
	wf, err := test.workflow()
	if err != nil {
		return err
	}

	// 2. load inputs
	in, err := test.input() // todo
	if err != nil {
		return err
	}

	// 3. collect tags
	tags := test.tags()

	// 4. make run request to mariner
	resp, err := r.requestRun(wf, in, tags)
	if err != nil {
		return err
	}

	// now, some conditional
	// if + test, then get run ID and wait for finish
	// (todo) if - test, maybe expect an error code resp from mariner server (?)

	// HERE (2/11/20 1:30pm) - now that you have the runID and mariner is running the test
	// hit status endpoint, wait for "completed" or "failed"
	// set a stopping rule on time - e.g., if "running" for five minutes, test failed
	// upon completion, match outputs, record test result
	//
	// mostly the tricky thing is just error handling..
	// handling exceptional control flow.

	// 4.5. get the runID
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	runID := &mariner.RunIDJSON{}
	if err = json.Unmarshal(b, runID); err != nil {
		return err
	}

	// 5. listen for done (or err/fail)

	// 6. match output

	// 7. save/record result

	return nil
}

func (r *Runner) requestRun(wf *wftool.WorkflowJSON, in map[string]interface{}, tags map[string]string) (*http.Response, error) {
	b, err := body(wf, in, tags)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", runEndpoint, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", r.Token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func body(wf *wftool.WorkflowJSON, in map[string]interface{}, tags map[string]string) ([]byte, error) {
	req := WorkflowRequest{
		Workflow: wf,
		Input:    in,
		Tags:     tags,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// printJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
