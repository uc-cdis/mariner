package conformance

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Runner ..
type Runner struct {
	Environment string      `json:"environment"`
	Token       string      `json:"-"`
	Timestamp   string      `json:"timestamp"`
	Duration    string      `json:"duration"`
	Async       *Async      `json:"async"`
	Results     *Counts     `json:"results"`
	Log         *ResultsLog `json:"log"`
}

// Async ..
type Async struct {
	Enabled       bool
	MaxConcurrent int
	NRunning      int            `json:"-"`
	InProgress    map[int]bool   `json:"-"`
	Mutex         sync.Mutex     `json:"-"`
	WaitGroup     sync.WaitGroup `json:"-"`
}

// Counts ..
type Counts struct {
	Total    int
	Coverage float64 // == #pass / #total
	Pass     int
	Fail     int
	Manual   int
}

// ResultsLog captures test results and mariner logs of each run
// fixme: capture/store expected output for each test -> basics. it's a field in struct TestCase
type ResultsLog struct {
	Pass map[int]*PassLog
	Fail map[int]*FailLog
	// Error map[int]error

	// guarding against false positives
	// some tests need to be looked at closely, at least for now
	Manual map[int]*ManualLog
}

// PassLog ..
type PassLog struct {
	TestCase *TestCase
	RunLog   *RunLog
}

// ManualLog ..
type ManualLog struct {
	TestCase *TestCase
	RunLog   *RunLog
}

// FailLog ..
type FailLog struct {
	TestCase        *TestCase
	TimeOut         bool
	FailedToKillJob bool // if timeout -> cancel job -> if err during job cancel, set to true
	LocalError      error
	MarinerError    []string
	RunLog          *RunLog
}

// Run ..
// Runner runs the given test and logs the test result
func (r *Runner) run(test *TestCase) (err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("runner panicked: %v", panicErr)
		}
	}()

	// for json marshalling purposes
	test.Output = convertInterface(test.Output)

	fmt.Printf("------ running test %v ------\n", test.ID)

	// pack the CWL to json (wf)
	fmt.Printf("--- %v - packing cwl to json\n", test.ID)
	wf, err := test.workflow()
	if err != nil {
		return err
	}

	// load inputs
	fmt.Printf("--- %v - loading inputs\n", test.ID)
	in, err := test.input()
	if err != nil {
		return err
	}

	// collect tags
	fmt.Printf("--- %v - collecting tags\n", test.ID)
	tags := test.tags()

	// make run request to mariner
	fmt.Printf("--- %v - POSTing request to mariner\n", test.ID)
	resp, err := r.requestRun(wf, in, tags)
	/*
		fixme: check if resp contains an error message
		e.g., this sequence:
		--- 1 - POSTing request to mariner
		--- 1 - marshalling this: failed to create workflow job: jobs.batch "mariner.2020-3-23-21-18-31" already exists
		--- 1 - marshalling RunID to json
		--- 1 - error: invalid character 'i' in literal false (expecting 'l')
	*/
	if err != nil {
		return err
	}

	// get the runID
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// fmt.Printf("--- %v - marshalling this: %v\n", test.ID, string(b))

	fmt.Printf("--- %v - marshalling RunID to json\n", test.ID)
	runID := &RunIDJSON{}
	if err = json.Unmarshal(b, runID); err != nil {
		return err
	}
	fmt.Printf("--- %v - runID: %v\n", test.ID, runID.RunID)

	// listen for done
	fmt.Printf("--- %v - waiting for run to finish\n", test.ID)
	status, err := r.waitForDone(test, runID)
	if err != nil {
		return err
	}

	// fetch complete mariner logs for the test
	runLog, err := r.fetchRunLog(runID)
	if err != nil {
		return err
	}

	fmt.Printf("--- %v - run status: %v\n", test.ID, status)

	// case handling for +/- tests
	var match bool
	switch {
	case !test.ShouldFail && status == "completed":
		fmt.Printf("--- %v - matching output\n", test.ID)
		match, err = r.matchOutput(test, runLog)
		if err != nil {
			return err
		}
		if match {
			r.pass(test, runLog)
		} else {
			r.fail(test, runLog, status)
		}

		// fixme: make status values constants
	case (!test.ShouldFail && status == "failed") || status == "timeout":
		r.fail(test, runLog, status)
	case test.ShouldFail:
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
		r.manual(test, runLog)
	}

	return err
}

// return whether desired and actual test output match
func (r *Runner) matchOutput(test *TestCase, runLog *RunLog) (bool, error) {
	res, err := test.matchOutput(runLog.Main.Output)
	if err != nil {
		return false, err
	}
	return res, nil
}

// return whether desired and actual test output match
// expecting this to not work as desired
func (t *TestCase) matchOutput(testOut map[string]interface{}) (bool, error) {
	// desired:	t.Output
	// actual:	testOut
	match := reflect.DeepEqual(t.Output, testOut)
	/*
		if match {
			fmt.Println("these are equal*")
		} else {
			fmt.Println("these are not equal*")
		}
		fmt.Println("expected:")
		printJSON(t.Output)
		fmt.Println("got:")
		printJSON(testOut)
	*/
	return match, nil
}

// wait for test run to complete or fail
func (r *Runner) waitForDone(test *TestCase, runID *RunIDJSON) (status string, err error) {
	done := false
	url := fmt.Sprintf(fstatusEndpt, r.Environment, runID.RunID)

	// timeout after timeLimit is reached
	// make this configurable (?) -> yes, do this
	timeLimit := 3 * time.Minute
	start := time.Now()

	for !done {
		status, err = r.status(url)
		if err != nil {
			return "", err
		}

		switch status {
		case "running":
			// do nothing
		case "completed":
			done = true
		case "failed":
			done = true
		default:
			// fmt.Println("unexpected status: ", status)
		}

		if time.Since(start) > timeLimit {
			return "timeout", nil
		}

		time.Sleep(3 * time.Second)
	}
	return status, nil
}

// WriteResults ..
func (r *Runner) WriteResults(outPath string) error {
	// if no outpath specified, write to default outpath
	if outPath == "" {
		outPath = fmt.Sprintf("conformance%v.json", r.Timestamp)
	}

	fmt.Printf("------ writing test results to %v ------\n", outPath)

	f, err := os.Create(outPath)
	defer f.Close()
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(f)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(r); err != nil {
		return err
	}
	return nil
}

func (r *Runner) error(test *TestCase, err error) error {
	fmt.Printf("--- %v - error: %v\n", test.ID, err)
	r.Log.Fail[test.ID] = &FailLog{
		TestCase:   test,
		LocalError: err,
	}
	return err
}

func (r *Runner) fail(test *TestCase, runLog *RunLog, status string) {
	log := &FailLog{
		TestCase:     test,
		RunLog:       runLog,
		MarinerError: []string{},
	}
	// collect all error messages from main mariner log
	for _, msg := range runLog.Main.Event {
		if strings.Contains(msg, "- ERROR -") {
			log.MarinerError = append(log.MarinerError, msg)
		}
	}
	if status == "timeout" {
		log.TimeOut = true

		// note: engine jobName is the runID - need to clarify this in all mariner code
		cancelEndpt := fmt.Sprintf(fcancelEndpt, r.Environment, runLog.Main.JobName)
		if err := r.cancelRun(cancelEndpt); err != nil {
			log.FailedToKillJob = true
			fmt.Printf("--- %v - failed to kill job: %v\n", test.ID, err)
		}
	}
	r.Log.Fail[test.ID] = log
}

func (r *Runner) pass(test *TestCase, runLog *RunLog) {
	r.Log.Pass[test.ID] = &PassLog{
		TestCase: test,
		RunLog:   runLog,
	}
}

func (r *Runner) manual(test *TestCase, runLog *RunLog) {
	r.Log.Manual[test.ID] = &ManualLog{
		TestCase: test,
		RunLog:   runLog,
	}
}
