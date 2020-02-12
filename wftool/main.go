package main

import (
	"flag"
	"fmt"

	"github.com/uc-cdis/mariner/wflib"
)

func main() {

	/*
		Usage:

		wftool -pack -i path/to/workflow.cwl -o myPackedWorkflow.json
		wftool -validate -i path/to/packedWorkflow.json
	*/

	var input, output string
	var pack, validate bool
	flag.StringVar(&input, "i", "", "path to workflow")
	flag.StringVar(&output, "o", "", "output path")
	flag.BoolVar(&pack, "pack", false, "pack and validate a CWL workflow")
	flag.BoolVar(&validate, "validate", false, "validate a packed CWL workflow (i.e., validate JSON)")

	flag.Parse()

	switch {
	case pack && validate:
		fmt.Println("command error - must specify exactly one of 'pack' or 'validate', not both")
	case !(pack || validate):
		fmt.Println("command error - must specify either 'pack' or 'validate'")
	case input == "":
		fmt.Println("command error - must specify input path")
	case pack:
		if err := wflib.Pack(input, output); err != nil {
			fmt.Println("pack operation failed due to error: ", err)
		}
	case validate:
		if valid, grievances := wflib.ValidateJSONFile(input); !valid {
			fmt.Println("invalid - see grievances:")
			wflib.PrintJSON(grievances)
		} else {
			fmt.Println("valid")
		}
	}
}
