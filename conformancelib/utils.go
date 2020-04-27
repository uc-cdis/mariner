package conformance

import (
	"encoding/json"
	"fmt"
)

// preprocesses arbitrarily nested struct/map for json marshalling
// in particular, converts map[interface{}]interface{} objects to map[string]interface{} objects
// without this preprocessing step, json encoder chokes due to unsupported type map[interface{}]interface{}
func convertInterface(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convertInterface(v)
		}
		return m2
	case map[string]interface{}:
		for k, v := range x {
			x[k] = convertInterface(v)
		}
	case []interface{}:
		for i, v := range x {
			x[i] = convertInterface(v)
		}
	}
	return i
}

// use reflect - do it right
// e.g., be able to handle []*TestCase
func nuConvert(i interface{}) interface{} {

	return nil
}

// printJSON pretty prints a struct as json
func printJSON(i interface{}) {
	i = convertInterface(i)
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
