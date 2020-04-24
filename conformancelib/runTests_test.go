package conformance

import (
	"fmt"
	"testing"
)

// incomplete
func NotTestFilter(t *testing.T) {
	suite, err := loadConfig(config)
	if err != nil {
		t.Errorf("failed to load tests")
	}

	// trueVal := true
	filters := &FilterSet{
		// ShouldFail: &trueVal,
		Tags:  []string{},
		Label: "",
		ID:    []int{},
	}

	fmt.Println("original length: ", len(suite))

	// apply filter to test list
	filtered := filters.apply(suite)

	fmt.Println("filtered length: ", len(filtered))

	fmt.Println("filters:")
	printJSON(filters)

	fmt.Println("filtered results:")
	printJSON(filtered)
}

// also incomplete
func NotTestInputsCollector(t *testing.T) {
	suite, err := loadConfig(config)
	if err != nil {
		t.Errorf("failed to load tests")
	}

	inputs, err := InputFiles(suite)

	fmt.Println("inputs:")
	printJSON(inputs)

	if err != nil {
		t.Errorf("collect routine failed: %v", err)
	}
}

/*

run stuck "running" forever
either the run is actually tatking that long
OR, it failed, but the error has not been handled/caught
just fail out, and log the error
should be able to access all this through the api
should not have to ssh into the pod to debug this

"panic" translates to a failed run
the job should never be stuck running forever because it errored out
code should not have the option of panic'ing out of control
at the highest level:
	- recover from panic
	- log the error
	- log the run failure
	- fail out/end the run
that the run failed should be reflected in the logs and the status
you should be able to get this simply from the API

Short List of Errors
0. don't panic (resolved)
1. non-unique k8s job names for two running instances of the same test case (resolved)

live text:
'''
loading input #main/args.py..
panic: runtime error: invalid memory address or nil pointer dereference
'''
pretty sure this is because this input param isn't specified in inputs.json
and currently the code doesn't handle default values
even though one is provided

SO

can support the default input value case right now

to handle:
1. default values
2. optional params
---> when to fail out?

*/

func TestRun(t *testing.T) {
	// goal: run 1 simple test, round trip - achieved!

	// load in all tests
	allTests, err := loadConfig(config)
	if err != nil {
		t.Errorf("failed to load tests")
	}

	// define filter
	filters := &FilterSet{
		ID: []int{13},
	}

	// how many tests to run
	/*
		nTests := 16
		for i := 1; i <= nTests; i++ {
			filters.ID = append(filters.ID, i)
		}
	*/

	// cap number of tests running at one time
	maxConcurrent := 4
	async := &Async{
		Enabled:       true,
		MaxConcurrent: maxConcurrent,
	}

	fmt.Println("nAllTests:", len(allTests))
	// fmt.Println("nTests:", nTests)
	fmt.Println("async:")
	printJSON(async)

	// apply filter
	tests := filters.apply(allTests)

	// look at the test set
	// fmt.Println("running these tests:")
	// printJSON(tests)

	// run the tests - results sent to stdout
	creds := "./creds.json"

	runner, err := RunTests(tests, creds, async)
	if err != nil {
		t.Error(err)
	}
	if err = runner.writeResults(""); err != nil {
		t.Error(err)
	}
}
