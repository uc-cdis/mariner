package main

import (
	"os"
	"testing"
)

const (
	noInputCWL         = "../testdata/no_input_test/workflow/cwl/gen3_test.cwl"
	noInputTargetJSON  = "../testdata/no_input_test/workflow/workflow.json"
	userDataCWL        = "../testdata/user_data_test/workflow/cwl/user-data_test.cwl"
	userDataTargetJSON = "../testdata/user_data_test/workflow/workflow.json"
)

/*
  Todo:
  1. write compareJSON()
  2. write fn TestValidate()
*/

// todo - read in each json file, compare
// return true if match, false if not match
func compareJSON(testPath string, targetPath string) bool {

	return true
}

func TestPack(t *testing.T) {
	// testDir, _ := os.Getwd()
	var match bool
	var err error
	out := "testPack.json"
	compare := func(cwl, target string) {
		// os.Chdir(testDir)
		if err = Pack(cwl, out); err != nil {
			t.Errorf("failed to pack cwl %v", cwl)
		}
		if match = compareJSON(out, target); !match {
			t.Errorf("%v packed json does match target json", cwl)
		}
		os.Remove(out)
	}
	compare(noInputCWL, noInputTargetJSON)
	compare(userDataCWL, userDataTargetJSON)
}
