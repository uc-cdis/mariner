package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	conformanceLib "github.com/uc-cdis/mariner/conformancelib"
)

var (
	trueVal  = true
	falseVal = false
)

func main() {
	// gives you pointers
	env := flag.String("env", "", "domain of target environment")
	cwl := flag.String("cwl", "./common-workflow-language", "path to the common-workflow-language repo")
	creds := flag.String("creds", "./creds.json", "path to creds (i.e., the api key json from the portal)")
	outPath := flag.String("out", "", "path to output json containing test results")
	maxConcurrent := flag.Int("async", 4, "specify maximum number of tests concurrently running at any given time")
	runTests := flag.Bool("run", false, "bool indicating whether the user wants to run the selected tests")

	// filter flags
	var labels FlagArrayString
	var tags FlagArrayString
	var IDs FlagArrayInt
	showFiltered := flag.Bool("showFiltered", false, "specify whether to send resulting set of test cases after filter to stdout")
	shouldFail := flag.Bool("neg", false, "if provided, then filter for negative test cases")
	shouldNotFail := flag.Bool("pos", false, "if provided, then filter for positive test cases")
	flag.Var(&labels, "lab", "comma-separated list of labels by which to filter the test cases")
	flag.Var(&tags, "tag", "comma-separated list of tags by which to filter the test cases")
	flag.Var(&IDs, "id", "comma-separated list of IDs by which to filter the test cases")

	flag.Parse()

	// load in all tests
	allTests, err := conformanceLib.LoadConfig(*cwl)
	if err != nil {
		fmt.Println("error loading tests:", err)
	}

	// define filter
	filters := &conformanceLib.FilterSet{
		ID:    []int(IDs),
		Label: []string(labels),
		Tags:  []string(tags),
	}
	switch {
	case *shouldFail && !*shouldNotFail:
		filters.ShouldFail = &trueVal
	case !*shouldFail && *shouldNotFail:
		filters.ShouldFail = &falseVal
	default:
		filters.ShouldFail = nil
	}

	// apply filter
	tests := filters.Apply(allTests)

	// optionally view filtered test set
	if *showFiltered {
		conformanceLib.PrintJSON(tests)
		fmt.Printf("--- nTests: %v ---\n", len(tests))
	}

	// optionally run filtered test set
	if *runTests {

		if *env == "" {
			fmt.Println("error: missing domain of target environment - cannot run tests without specifying target domain")
			return
		}

		fmt.Printf("\n--- running %v tests ---\n", len(tests))

		// cap number of tests running at one time
		async := &conformanceLib.Async{
			Enabled:       true,
			MaxConcurrent: *maxConcurrent,
		}

		fmt.Println("--- async settings: ---")
		conformanceLib.PrintJSON(async)
		fmt.Println("")

		// run the tests
		runner, err := conformanceLib.RunTests(tests, *env, *creds, async)
		if err != nil {
			fmt.Println("error running tests:", err)
		}
		if err = runner.WriteResults(*outPath); err != nil {
			fmt.Println("error writing results:", err)
		}
	}
}

// FlagArrayString ..
type FlagArrayString []string

// String ..
func (f *FlagArrayString) String() string {
	return fmt.Sprint(*f)
}

// Set ..
func (f *FlagArrayString) Set(value string) error {
	for _, s := range strings.Split(value, ",") {
		*f = append(*f, s)
	}
	return nil
}

// FlagArrayInt ..
type FlagArrayInt []int

// String ..
func (f *FlagArrayInt) String() string {
	return fmt.Sprint(*f)
}

// Set ..
func (f *FlagArrayInt) Set(value string) error {
	for _, s := range strings.Split(value, ",") {
		i, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*f = append(*f, i)
	}
	return nil
}
