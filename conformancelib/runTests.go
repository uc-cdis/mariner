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
func RunTests(tests []*TestCase, creds string, async *Async) (*Runner, error) {

	tok, err := token(creds)
	if err != nil {
		return nil, err
	}

	runner := NewRunner(tok, async)

	start := time.Now()
	runner.runTests(tests)
	runner.Duration = time.Since(start).Truncate(1 * time.Second).String()

	return runner, nil
}

func (r *Runner) runTests(tests []*TestCase) {
	var err error
	for _, test := range tests {

		switch {
		case r.Async.Enabled:
			// run tests concurrently
		default:
			if err = r.run(test); err != nil {
				r.logError(test, err)
			}
		}

	}
	r.tally()
}

func (r *Runner) tally() {
	r.Results.Pass = len(r.Log.Pass)
	r.Results.Fail = len(r.Log.Error) + len(r.Log.Fail)
	r.Results.Manual = len(r.Log.Manual)
	r.Results.Total = r.Results.Pass + r.Results.Fail + r.Results.Manual
	r.Results.Coverage = float64(r.Results.Pass) / float64(r.Results.Total)
}

// NewRunner ..
func NewRunner(tok string, async *Async) *Runner {
	r := &Runner{
		Token:     tok,
		Log:       new(ResultsLog),
		Results:   new(Counts),
		Timestamp: time.Now().Format("010206150405"),
		Async:     async,
	}
	r.Log.Pass = make(map[int]*RunLog)
	r.Log.Fail = make(map[int]*RunLog)
	r.Log.Manual = make(map[int]*RunLog)
	r.Log.Error = make(map[int]error)
	return r
}
