package wftool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// error handling, in general, needs attention

// PackCWL serializes a single cwl byte to json
func PackCWL(cwl []byte, id string) {
	cwlObj := new(interface{})
	yaml.Unmarshal(cwl, cwlObj)
	*cwlObj = nuConvert(*cwlObj, primaryRoutine, id, false)
	printJSON(cwlObj)
}

func packCWLFile(path string) error {
	cwl, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	// copying cwltool's pack id scheme
	// not sure if it's actually good or not
	// but for now, doing this
	id := fmt.Sprintf("#%v", filepath.Base(path))
	PackCWL(cwl, id)
	return nil
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
