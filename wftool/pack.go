package wftool

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"
)

// pack serializes a single cwl file to json
func pack(cwl []byte) {
	cwlObj := new(CWLObject)
	yaml.Unmarshal(cwl, cwlObj)
	fmt.Println(cwlObj)
}

// PrintJSON pretty prints a struct as json
func printJSON(i interface{}) {
	var see []byte
	var err error
	see, err = json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v", err)
	}
	fmt.Println(string(see))
}
