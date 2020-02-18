package conformance

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
)

/*
Short list (2/10/19):
2. need to collect all file inputs so to stage in s3
*/

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

// Run ..
// Runner runs the given test and logs the test result
func (r *Runner) Run(test *TestCase) error {

	// 1. pack the CWL to json (wf)
	wf, err := test.workflow()
	if err != nil {
		fmt.Println("failed at workflow()")
		// fmt.Printf("%v\n\n", test)
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

// log result of run of given TestCase
func (r *Runner) logResult(test *TestCase, match bool) {
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

// return whether desired and actual test output match
func (r *Runner) matchOutput(test *TestCase, runID *RunIDJSON) (bool, error) {
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

// return whether desired and actual test output match
// expecting this to not work as desired
func (t *TestCase) matchOutput(testOut map[string]interface{}) (bool, error) {
	// desired:	t.Output
	// actual:	testOut
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

// wait for test run to complete or fail
func (r *Runner) waitForDone(test *TestCase, runID *RunIDJSON) error {
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
