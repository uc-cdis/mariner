package conformance

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"time"
)

// Runner ..
type Runner struct {
	Token     string   `json:"-"`
	Timestamp string   `json:"timestamp"`
	Duration  string   `json:"duration"`
	Results   *Results `json:"results"`
}

// Results captures test results and mariner logs of each run
type Results struct {
	Pass  map[int]*RunLog
	Fail  map[int]*RunLog
	Error map[int]error

	// guarding against false positives
	// some tests need to be looked at closely, at least for now
	Manual map[int]*RunLog
}

// Run ..
// Runner runs the given test and logs the test result
func (r *Runner) run(test *TestCase) (err error) {
	defer func() {
		if panicErr := recover(); panicErr != nil {
			err = fmt.Errorf("runner panicked: %v", panicErr)
			r.Results.Error[test.ID] = err
		}
	}()

	fmt.Printf("------ running test %v ------\n", test.ID)

	// pack the CWL to json (wf)
	fmt.Println("--- packing cwl to json")
	wf, err := test.workflow()
	if err != nil {
		return r.logError(test, err)
	}

	// HERE - write error logger that returns err

	// load inputs
	fmt.Println("--- loading inputs")
	in, err := test.input()
	if err != nil {
		return r.logError(test, err)
	}

	// collect tags
	fmt.Println("--- collecting tags")
	tags := test.tags()

	// make run request to mariner
	fmt.Println("--- POSTing request to mariner")
	resp, err := r.requestRun(wf, in, tags)
	if err != nil {
		return r.logError(test, err)
	}

	// get the runID
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return r.logError(test, err)
	}
	runID := &RunIDJSON{}
	if err = json.Unmarshal(b, runID); err != nil {
		return r.logError(test, err)
	}
	fmt.Println("--- runID:", runID.RunID)

	// listen for done
	fmt.Println("--- waiting for run to finish")
	status, err := r.waitForDone(test, runID)
	if err != nil {
		return r.logError(test, err)
	}

	// fetch complete mariner logs for the test
	runLog, err := r.fetchRunLog(runID)
	if err != nil {
		return r.logError(test, err)
	}

	fmt.Println("--- run status:", status)

	// case handling for +/- tests
	var match bool
	switch {
	case !test.ShouldFail && status == "completed":
		// match output
		fmt.Println("--- matching output")
		match, err = r.matchOutput(test, runLog)
		if err != nil {
			return r.logError(test, err)
		}

		if match {
			r.Results.Pass[test.ID] = runLog
		} else {
			r.Results.Fail[test.ID] = runLog
		}

	case !test.ShouldFail && status == "failed":
		r.Results.Fail[test.ID] = runLog
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
		r.Results.Manual[test.ID] = runLog
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
	endpt := fmt.Sprintf(fstatusEndpt, runID.RunID)
	for !done {
		status, err = r.status(endpt)
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

		time.Sleep(3 * time.Second)
	}
	return status, nil
}

func (r *Runner) writeResults(outPath string) error {

	// if no outpath specified, write to default outpath
	if outPath == "" {
		outPath = fmt.Sprintf("conformance%v.json", r.Timestamp)
	}

	fmt.Printf("------ writing test results to %v ------\n", outPath)

	b, err := json.MarshalIndent(r, "", "    ")
	if err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	return nil
}

func (r *Runner) logError(test *TestCase, err error) error {
	r.Results.Error[test.ID] = err
	return err
}
