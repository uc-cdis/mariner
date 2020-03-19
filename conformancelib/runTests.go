package conformance

import (
	"time"
)

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
func RunTests(tests []*TestCase, creds string) (*Runner, error) {

	tok, err := token(creds)
	if err != nil {
		return nil, err
	}

	runner := NewRunner(tok)

	start := time.Now()
	runner.runTests(tests)
	runner.Duration = time.Since(start).String()

	return runner, nil
}

func (r *Runner) runTests(tests []*TestCase) {
	var err error
	for _, test := range tests {

		// dev with sequential, then make concurrent (?)
		// go runTest(test, tok)

		if err = r.run(test); err != nil {
			r.logError(test, err)
		}
	}
}

// NewRunner ..
func NewRunner(tok string) *Runner {
	r := &Runner{
		Token:     tok,
		Results:   new(Results),
		Timestamp: time.Now().Format("010206150405"),
	}
	r.Results.Pass = make(map[int]*RunLog)
	r.Results.Fail = make(map[int]*RunLog)
	r.Results.Manual = make(map[int]*RunLog)
	r.Results.Error = make(map[int]error)
	return r
}
