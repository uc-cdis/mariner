package conformance

/*
Want to run a subset of tests

Apply filters to the test suite

Filter on these fields:
	- ShouldFail (+/-)		-shouldFail		bool
	- Label					-label			string
	- ID					-id				[]int
	- Tags					-tags			[]string

Write a function to filter the tests

This will be a standalone feature of the CLI, a decoupled function

send filter results to stdout:
"conformance -filter <filter_flags>"

apply filter and run resulting test set:
"conformance -filter <filter_flags> -runTests -creds path/to/creds.json"
// sends results to stdout (and/or write to file?)
*/

// FilterSet ..
// maps to fields of a record in the testSuite.yaml config list of tests
type FilterSet struct {
	ShouldFail *bool    // not given --> nil
	Label      string   // not given --> ""
	ID         []int    // not given --> []
	Tags       []string // not given --> []
}

// given a populated FilterSet, filter these tests
func (f *FilterSet) apply(tests []*TestCase) []*TestCase {

	// if no filters specified, then return unfiltered list
	if f.ShouldFail == nil && f.Label == "" && len(f.ID) == 0 && len(f.Tags) == 0 {
		return tests
	}

	// at least one filter is specified, so we return a subset of the original test list
	out := []*TestCase{}
	var pass bool
	for _, test := range tests {
		if pass = f.check(test); pass {
			out = append(out, test)
		}
	}
	return out
}

// checks whether or not this test passes through the filter
func (f *FilterSet) check(test *TestCase) bool {
	if f.ShouldFail != nil {
		if test.ShouldFail == *f.ShouldFail {
			return true
		}
	}
	if f.Label != "" && f.Label == test.Label {
		return true
	}
	if len(f.ID) > 0 {
		for _, id := range f.ID {
			if test.ID == id {
				return true
			}
		}
	}
	if len(f.Tags) > 0 {
		for _, filterTag := range f.Tags {
			for _, testTag := range test.Tags {
				if filterTag == testTag {
					return true
				}
			}
		}
	}
	return false
}
