package main

import (
	"os"
	"testing"
)

const (
	noInputCWL  = "../testdata/no_input_test/workflow/cwl/gen3_test.cwl"
	userDataCWL = "../testdata/user_data_test/workflow/cwl/user-data_test.cwl"
)

func TestPack(t *testing.T) {
	var err error
	out := "testPack.json"
	p := func(cwl string) {
		if err = Pack(cwl, out); err != nil {
			t.Errorf("failed to pack cwl %v\nerror: %v", cwl, err)
		}
		if err = os.Remove(out); err != nil {
			t.Errorf("failed to cleanup packed cwl file\nerror: %v", err)
		}
	}
	p(noInputCWL)
	p(userDataCWL)
}
