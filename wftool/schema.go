package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

/*
	this tool should answer, to some degree, the question - "will this cwl run?"
	"will/should mariner even attempt to run this workflow?"

	validation
	for mariner - "should we dispatch the engine job for this workflow?
	or just return an error to the user now"
	for user - "will mariner run this workflow,
	or is there something I need to fix in my CWL?"
*/

/*
	currently with the way the cwl.go library is setup,
	certain fields need to be converted from map to array structure
	could make changes in cwl.go lib
	but first just getting things working
*/
var mapToArray = map[string]bool{
	"inputs":       true,
	"steps":        true,
	"in":           true,
	"outputs":      true,
	"requirements": true,
	"hints":        true,
}

func array(m map[interface{}]interface{}, parentKey string, parentID string, path string, graph *[]map[string]interface{}, versionCheck map[string][]string) []map[string]interface{} {
	arr := []map[string]interface{}{}
	var nuV map[string]interface{}
	for k, v := range m {
		id := fmt.Sprintf("%v/%v", parentID, k.(string))
		i := nuConvert(v, k.(string), id, false, path, graph, versionCheck)
		switch x := i.(type) {
		case map[string]interface{}:
			nuV = x
		case string:
			nuV = make(map[string]interface{})
			// handle shorthand syntax which is in the CWL spec
			switch parentKey {
			case "inputs", "outputs":
				nuV["type"] = resolveType(x)
			case "in":
				nuV["source"] = fmt.Sprintf("%v/%v", strings.Split(parentID, "/")[0], x)
			default:
				panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
			}
		default:
			panic(fmt.Sprintf("unexpected syntax for field: %v", parentKey))
		}
		switch parentKey {
		case "requirements", "hints":
			nuV["class"] = k.(string)
		default:
			nuV["id"] = id
		}
		arr = append(arr, nuV)
	}
	return arr
}

// currently only supporting base case - expecting string
// i.e., not supporting user-defined schemas or $import or $include or custom types
func resolveType(s string) interface{} {
	switch {
	case strings.HasSuffix(s, "[]"):
		return map[string]string{
			"type":  "array",
			"items": strings.TrimSuffix(s, "[]"),
		}
	case strings.HasSuffix(s, "?"):
		return []string{
			strings.TrimSuffix(s, "?"),
			"null",
		}
	}
	return s
}

const primaryRoutine = "primaryRoutine"

/*
consider separation of powers between cwl.go and this package
should they be the same package?
*/
func nuConvert(i interface{}, parentKey string, parentID string, inArray bool, path string, graph *[]map[string]interface{}, versionCheck map[string][]string) interface{} {
	/*
		fmt.Println("parentKey: ", parentKey)
		fmt.Println("object:")
		printJSON(i)
	*/
	switch x := i.(type) {
	case map[interface{}]interface{}:
		if mapToArray[parentKey] && !inArray {
			return array(x, parentKey, parentID, path, graph, versionCheck)
		}
		m2 := map[string]interface{}{}
		for k, v := range x {
			key := k.(string)
			m2[key] = nuConvert(v, key, parentID, false, path, graph, versionCheck)
		}
		// per cwl file
		// one initial call to nuConvert()
		// this initial call is the primaryRoutine
		// indicates we must populate the id field here
		if parentKey == primaryRoutine {
			m2["id"] = parentID
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = nuConvert(v, parentKey, parentID, true, path, graph, versionCheck)
		}
	case string:
		switch parentKey {
		case "cwlVersion":
			// collect paths corresponding to cwlVersions appearing in workflow
			// so when someone's workflow fails to pack because of conflicting versions
			// they can see which files they need to change
			versionCheck[x] = append(versionCheck[x], path)
		case "type":
			return resolveType(x)
		case "source", "outputSource":
			return fmt.Sprintf("%v/%v", strings.Split(parentID, "/")[0], x)
		case "out", "id", "scatter":
			return fmt.Sprintf("%v/%v", parentID, x)
		case "run":
			// this is not graceful, but is functionally sufficient for a first iteration
			if err := PackCWLFile(x, path, graph, versionCheck); err != nil {
				panic(fmt.Errorf("failed to pack file at path: %v\nparent path: %v", x, path))
			}
			return fmt.Sprintf("#%v", filepath.Base(x))
		}
	}
	return i
}
