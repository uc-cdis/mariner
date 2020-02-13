package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/uc-cdis/mariner/wflib"
)

// TestCase ..
type TestCase struct {
	Input      string                 `json:"job" yaml:"job"`                 // path to input.json (may also be yaml)
	Output     map[string]interface{} `json:"output" yaml:"output"`           // expected output
	ShouldFail bool                   `json:"should_fail" yaml:"should_fail"` // if the engine is expected to fail on this cwl
	CWL        string                 `json:"tool" yaml:"tool"`               // path to tool.cwl
	Label      string                 `json:"label" yaml:"label"`
	ID         int                    `json:"id" yaml:"id"`
	Doc        string                 `json:"doc" yaml:"doc"`
	Tags       []string               `json:"tags" yaml:"tags"`
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
	Error  map[int]error
	Manual []int // some tests need to be looked at closely, at least for now
	// guarding against false positives
}

// RunIDJSON ..
type RunIDJSON struct {
	RunID string `json:"runID"`
}

func main() {
	return
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
	runEndpt = "https://mattgarvin1.planx-pla.net/ga4gh/wes/v1/runs"

	fstatusEndpt = "https://mattgarvin1.planx-pla.net/ga4gh/wes/v1/runs/%v/status"
	flogsEndpt   = "https://mattgarvin1.planx-pla.net/ga4gh/wes/v1/runs/%v"

	inputPathPrefix = "USER/conformanceTesting/"
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

	r := Runner{
		Token:   tok,
		Results: new(Results),
	}
	r.Results.Error = make(map[int]error)

	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		// go runTest(test, tok)

		// dev with sequential, then make concurrent (?)
		if err = r.Run(test); err != nil {
			fmt.Println("err running test: ", err)
			r.Results.Error[test.ID] = err
		}
	}

	// for now
	/*
		fmt.Println("here are the results:")
		printJSON(r.Results)
	*/

	fmt.Println("fin")

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
2. need to collect all file inputs so to stage in s3
*/

// WorkflowRequest ..
type WorkflowRequest struct {
	Workflow *wflib.WorkflowJSON
	Input    map[string]interface{}
	Tags     map[string]string
}

func (t *TestCase) workflow() (*wflib.WorkflowJSON, error) {
	wf, err := wflib.PackWorkflow(t.CWL)
	if err != nil {
		return nil, err
	}
	valid, grievances := wflib.ValidateWorkflow(wf)
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

func (t *TestCase) input() (map[string]interface{}, error) {
	ext := filepath.Ext(t.Input)
	if ext != ".json" && ext != ".yaml" {
		return nil, fmt.Errorf("unexpected inputs fileext: %v", ext)
	}

	b, err := ioutil.ReadFile(t.Input)
	if err != nil {
		return nil, err
	}

	in := &map[string]interface{}{}
	switch ext {
	case ".json":
		err = json.Unmarshal(b, in)
	case ".yaml":
		err = yaml.Unmarshal(b, in)
	}
	if err != nil {
		return nil, err
	}

	input := addPathPrefix(*in)

	return input, nil
}

// affix the prefix
func addPathPrefix(in map[string]interface{}) map[string]interface{} {
	var f map[string]interface{}
	var ok bool
	var path string
	for _, v := range in {
		if f, ok = v.(map[string]interface{}); ok && f["class"].(string) == "File" {
			if path = f["location"].(string); path != "" {
				f["location"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
			}
			if path = f["path"].(string); path != "" {
				f["path"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
			}
		}
	}
	return in
}

// Run ..
// run the test and record test result in the runner
func (r *Runner) Run(test TestCase) error {

	// fmt.Println(test)

	// make these fns methods of type TestCase

	// 1. pack the CWL to json (wf)
	wf, err := test.workflow()
	if err != nil {
		fmt.Println("failed at workflow()")
		fmt.Printf("%v\n\n", test)
		return err
	}

	// 2. load inputs
	in, err := test.input()
	if err != nil {
		fmt.Println("failed at input()")
		return err
	}

	// 3. collect tags
	tags := test.tags()

	// 4. make run request to mariner
	resp, err := r.requestRun(wf, in, tags)
	if err != nil {
		fmt.Println("failed at requestRun()")
		return err
	}

	// 4.5. get the runID
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	runID := &RunIDJSON{}
	if err = json.Unmarshal(b, runID); err != nil {
		return err
	}

	// 5. listen for done
	err = r.waitForDone(test, runID)

	// 6. match output
	match, err := r.matchOutput(test, runID)
	if err != nil {
		return err
	}

	// 7. save/record result
	r.logResult(test, match)

	return nil
}

func (r *Runner) logResult(test TestCase, match bool) {
	/*
		currently flagging all negative test cases as manual checks
		not sure where or exactly how the engine should fail
		e.g.,
		given a negative test, the run could fail:
		1. at wf validation
			i.e., when it is packed,
			and/or when the run request is POSTed to mariner server
		2. the job may dispatch but fail mid-run
			i.e., status during r.waitForDone() should reach "failed"
		3. the job may run to completion but return nothing or the incorrect output

		so, until I figure out where/how to check
		that the negative test cases are failing as expected
		they will be flagged to be checked manually

		I believe there are only a handful of them anyway
	*/
	switch {
	case !test.ShouldFail && match:
		r.Results.Pass = append(r.Results.Pass, test.ID)
	case !test.ShouldFail && !match:
		r.Results.Fail = append(r.Results.Fail, test.ID)
	case test.ShouldFail:
		r.Results.Manual = append(r.Results.Manual, test.ID)
	}
}

func (r *Runner) matchOutput(test TestCase, runID *RunIDJSON) (bool, error) {
	out, err := r.output(runID)
	if err != nil {
		return false, err
	}
	res, err := test.matchOutput(out)
	if err != nil {
		return false, err
	}
	return res, nil
}

// expecting this to not work as desired
func (t *TestCase) matchOutput(testOut map[string]interface{}) (bool, error) {
	match := reflect.DeepEqual(t.Output, testOut)
	fmt.Println("-----------------")
	if match {
		fmt.Println("these are equal*")
	} else {
		fmt.Println("these are not equal*")
	}
	fmt.Println("expected:")
	printJSON(t.Output)
	fmt.Println("got:")
	printJSON(testOut)
	return match, nil
}

// RunLog ..
type RunLog struct {
	// Path      string           `json:"path"` // tentative  - maybe can't write this - path to log file to write/update
	// Request   *WorkflowRequest `json:"request"`
	Main *Log `json:"main"`
	// ByProcess map[string]*Log  `json:"byProcess"`
}

// Log ..
type Log struct {
	/*
		Created        string                 `json:"created,omitempty"`     // okay - timezone???
		CreatedObj     time.Time              `json:"-"`                     // okay
		LastUpdated    string                 `json:"lastUpdated,omitempty"` // okay - timezone???
		LastUpdatedObj time.Time              `json:"-"`                     // okay
		JobID          string                 `json:"jobID,omitempty"`       // okay
		JobName        string                 `json:"jobName,omitempty"`     // keeping for now, but might be redundant w jobID
		Status         string                 `json:"status"`                // okay
		Stats          *Stats                 `json:"stats"`                 // TODO
		Event          EventLog               `json:"eventLog,omitempty"`    // TODO
		Input          map[string]interface{} `json:"input"`                 // TODO for workflow; okay for task
		Scatter        map[int]*Log           `json:"scatter,omitempty"`
	*/
	Output map[string]interface{} `json:"output"` // okay
}

// StatusJSON ..
type StatusJSON struct {
	Status string `json:"status"`
}

func (r *Runner) output(runID *RunIDJSON) (map[string]interface{}, error) {
	url := fmt.Sprintf(flogsEndpt, runID.RunID)
	resp, err := r.request("GET", url, nil)
	if err != nil {
		return nil, err
	}

	log := &RunLog{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, log)
	if err != nil {
		return nil, err
	}
	return log.Main.Output, nil
}

func (r *Runner) waitForDone(test TestCase, runID *RunIDJSON) error {
	done := false
	endpt := fmt.Sprintf(fstatusEndpt, runID.RunID)
	for !done {
		status, err := r.status(endpt)
		if err != nil {
			return err
		}

		switch status {
		case "completed":
			done = true
		case "running":
		case "failed":
			// this may or may not be an error
			// in the case of a negative test
			return fmt.Errorf("run failed")
		}
	}
	return nil
}

func (r *Runner) status(url string) (string, error) {
	resp, err := r.request("GET", url, nil)
	if err != nil {
		return "", err
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	s := &StatusJSON{}
	if err = json.Unmarshal(b, s); err != nil {
		return "", err
	}
	return s.Status, nil
}

func (r *Runner) requestRun(wf *wflib.WorkflowJSON, in map[string]interface{}, tags map[string]string) (*http.Response, error) {
	b, err := body(wf, in, tags)
	if err != nil {
		return nil, err
	}

	resp, err := r.request("POST", runEndpt, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// add auth header, make request, return response
func (r *Runner) request(method string, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
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

func body(wf *wflib.WorkflowJSON, in map[string]interface{}, tags map[string]string) ([]byte, error) {
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

func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	}
	return i
}

// printJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	i = convert(i)
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
