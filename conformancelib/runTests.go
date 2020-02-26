package conformance

import "fmt"

/*
what are all the required params to run the tests?

need:
	-path/to/creds.json (apiKey)
	-path/to/testSuite.yaml (conformance_test_v1.0.yaml)
	-path/to/dirWithCWLandInputParamsAndFiles ('tool' and 'job' fields for each test)

it's just a conversation with mariner.

given: all input files have been staged to the test user's user data space
	- i.e., all data files are located here: s3://USERDATA/userID/conformanceTesting/

you need:
1. an apiKey
2. a list of tests
3. the cwl for each test
4. the input params for each test

the set of flags for the cli and their definitions just need to cover this space

e.g.,

send filter results to stdout:
"conformance -filter <filter_flags>"

apply filter and run resulting test set:
"conformance -filter <filter_flags> -runTests -creds path/to/creds.json"
// sends results to stdout (and/or write to file?)

send list of input files to stdout (optional filter flag):
"conformance -listInput"
*/

// RunTests ..
// 'creds' is path/to/creds.json which is what you get
// when you create and download an apiKey from the portal
func RunTests(tests []*TestCase, creds string) error {

	tok, err := token(creds)
	if err != nil {
		return err
	}
	r := NewRunner(tok)
	r.runTests(tests)

	// for now
	fmt.Println("here are the results:")
	printJSON(r.Results)

	fmt.Println("fin")

	return nil
}

func (r *Runner) runTests(tests []*TestCase) error {
	var err error
	for _, test := range tests {
		// could make a channel to capture errors from individual tests
		// go runTest(test, tok)

		// dev with sequential, then make concurrent (?)
		if err = r.run(test); err != nil {
			fmt.Println("err running test: ", err)
			r.Results.Error[test.ID] = err
		}
	}
	return nil
}

// NewRunner ..
func NewRunner(tok string) *Runner {
	r := &Runner{
		Token:   tok,
		Results: new(Results),
	}
	r.Results.Error = make(map[int]error)
	return r
}