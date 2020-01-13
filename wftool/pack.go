package wftool

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"
)

// Pack serializes a single cwl file to json
func Pack(cwl []byte) {
	// cwlObj := new(CommandLineTool)
	cwlObj := new(interface{})
	yaml.Unmarshal(cwl, cwlObj)
	fmt.Printf("%T\n", *cwlObj)
	fmt.Printf("%#v\n", *cwlObj)

	*cwlObj = nuConvert(*cwlObj, "")

	fmt.Println("here's the struct in JSON:")
	printJSON(cwlObj)

	/*
		j, err := cwlObj.JSON()
		if err != nil {
			fmt.Println("error marshalling to json: ", err)
		}
		fmt.Println("got this json:")
		printJSON(j)
	*/

}

// PrintJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	var see []byte
	var err error
	see, err = json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v", err)
	}
	fmt.Println(string(see))
}
