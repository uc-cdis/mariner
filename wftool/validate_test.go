package main

import (
	"testing"
)

func TestValidate(t *testing.T) {
	l := []string{
		noInputTargetJSON,
		userDataTargetJSON,
	}
	for _, f := range l {
		// valid, grievances := ValidateJSONFile(f)
		valid, _ := ValidateJSONFile(f)
		if !valid {
			// fmt.Println("grievances: ")
			// printJSON(grievances)
			t.Errorf("%v failed validation", f)
		}
	}
}
