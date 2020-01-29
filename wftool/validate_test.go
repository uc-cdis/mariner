package main

import (
	"fmt"
	"testing"
)

const (
	// positive test cases
	userDataTargetJSON = "../testdata/user_data_test/workflow/workflow.json"
	noInputTargetJSON  = "../testdata/no_input_test/workflow/workflow.json"

	// negative test cases
	n1 = ``
	n2 = `{}`
	n3 = `{"$graph": [{}]}`
	n4 = `{"cwlVersion": "v1.0"}`
	n5 = `{
		"$graph": {},
		"cwlVersion": "v1.0"
	}`
)

var pos = []string{userDataTargetJSON, noInputTargetJSON}
var neg = []string{n1, n2, n3, n4, n5}

func TestValidate(t *testing.T) {
	var valid bool
	var g *WorkflowGrievances
	for _, f := range pos {
		valid, g = ValidateJSONFile(f)
		if !valid {
			fmt.Println("validation grievances: ")
			printJSON(g)
			t.Errorf("%v failed validation", f)
		}
	}

	for _, j := range neg {
		g = &WorkflowGrievances{}
		if valid, g = ValidateJSON([]byte(j), g); valid {
			t.Errorf("negative test case passed validation:\n%v", j)
		}
	}
}
