package main

import (
	"fmt"
	"testing"
)

func TestValidate(t *testing.T) {
	// wd, _ := os.Getwd()
	// fmt.Println("wd: ", wd)
	l := []string{
		noInputTargetJSON,
		userDataTargetJSON,
	}
	for _, f := range l {
		valid, grievances := ValidateJSONFile(f)
		// valid, _ := ValidateJSONFile(f)
		if !valid {
			fmt.Println("grievances: ")
			printJSON(grievances)
			t.Errorf("%v failed validation", f)
		}
	}
}
