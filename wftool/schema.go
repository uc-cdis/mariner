package wftool

import (
	"fmt"
	"strings"
)

/*
	will this tool just marshal without enforcing/validating the cwl?
	e.g., if scatter, then scattermethod - will we perform that check here?
	or does this tool assume your cwl is error-free
	probably this tool should have some kind of validation function
	this tool should answer, to some degree, the question - "will this cwl run?"
	"will/should mariner even attempt to run this workflow?"

	validation
	for mariner - "should we dispatch the engine job for this workflow?
	or just return an error to the user now"
	for user - "will mariner run this workflow,
	or is there something I need to fix in my CWL?"

	need to handle type:
	'type' key maps to either <T> or []<T>
	handle <T> vs. []<T>
	if []<T>:
		for each <T>,
			if <T>.endsWith("[]") {
				saladArray(<T>)
			}
			elif <T>.endsWith("?") {
				saladOptional(<T>)
			}
*/

// original
func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	}
	return i
}

/*
	certain fields need to be converted from map to array structure
	currently, the way the cwl.go library is setup
	could make changes there
	but first just getting things working
*/
var mapToArray = map[string]bool{
	"inputs":       true,
	"outputs":      true,
	"requirements": true,
	"hints":        true,
}

func array(m map[interface{}]interface{}, parentKey string) []map[string]interface{} {
	arr := []map[string]interface{}{}
	var nuV map[string]interface{}
	for k, v := range m {
		i := nuConvert(v, k.(string))
		switch x := i.(type) {
		case map[string]interface{}:
			nuV = x
		case string:
			// if inputs, where you have id: type
			// not expecting any other instances of this
			nuV = make(map[string]interface{})
			if parentKey == "inputs" {
				nuV["type"] = x
			} else {
				panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
			}
		default:
			panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
		}
		switch parentKey {
		case "requirements", "hints":
			nuV["class"] = k.(string)
		default:
			nuV["id"] = k.(string)
		}
		arr = append(arr, nuV)
	}
	return arr
}

// currently only supporting base case - expecting string
// i.e., not supporting user-defined schemas or $import or $include or custom types
func resolveType(s string) interface{} {
	var out interface{}
	switch {
	case strings.HasSuffix(s, "[]"):
		out = map[string]string{
			"type":  "array",
			"items": strings.TrimSuffix(s, "[]"),
		}
	case strings.HasSuffix(s, "?"):
		out = []string{
			strings.TrimSuffix(s, "?"),
			"null",
		}
	}
	return out
}

/*
HERE - TODO - finish this fn - does all the work

consider: separation of powers between cwl.go and this package
should they be the same package?

*/
func nuConvert(i interface{}, parentKey string) interface{} {
	fmt.Println("handling field: ", parentKey)
	switch x := i.(type) {
	case map[interface{}]interface{}:
		switch {
		case mapToArray[parentKey]:
			return array(x, parentKey)
			// case ..
		}

		m2 := map[string]interface{}{}
		for k, v := range x {
			key := k.(string)
			m2[key] = nuConvert(v, key)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = nuConvert(v, "")
		}
	case string:
		if parentKey == "type" {
			return resolveType(x)
		}
	}
	return i
}
