package main

import (
	"flag"
	"fmt"
)

func main() {

	var input, output string
	flag.StringVar(&input, "i", "", "path to workflow")
	flag.StringVar(&output, "o", "", "output path")

	flag.Parse()

	// HERE todo
	cmd := flag.Arg(0)
	switch cmd {
	case "pack":
		err := Pack(input, output)
		if err != nil {
			fmt.Println("pack operation failed due to error: ", err)
		}
	case "validate":
		valid, grievances := validateJSONFile(input)
		if !valid {
			fmt.Println("invalid json - see grievances:")
			printJSON(grievances)
		}
	}
}
