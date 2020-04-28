package conformance

import (
	"encoding/json"
	"fmt"
)

// preprocesses *somewhat* arbitrarily nested struct/map for json marshalling
// in particular, converts map[interface{}]interface{} objects to map[string]interface{} objects
// without this preprocessing step, json encoder chokes due to unsupported type map[interface{}]interface{}
func convertInterface(i interface{}) interface{} {
	// debug
	// fmt.Printf("\nconverting %T\n%v\n", i, i)
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
	case []*TestCase:
		a := make([]interface{}, len(x))
		for i, v := range x {
			a[i] = convertInterface(*v)
		}
		return a
	case TestCase:
		x.Output = convertInterface(x.Output)
		return x
	}
	return i
}

// PrintJSON pretty prints a struct as json
func PrintJSON(i interface{}) {
	i = convertInterface(i)
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n\n%T\n", err, i)
	}
	fmt.Println(string(see))
}
