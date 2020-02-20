package conformance

import (
	"fmt"
	"reflect"
)

// InputsCollector collects paths of input parameters of type File
type InputsCollector struct {
	Collected map[string]bool
}

// given a test list
// return the list of files which must be staged
// -- to the target test environment
// -- in order for the tests to run as expected
func inputFiles(tests []*TestCase) ([]string, error) {
	var inputs map[string]interface{}
	var err error

	collector := &InputsCollector{
		Collected: make(map[string]bool),
	}

	for _, test := range tests {
		inputs, err = test.input()
		if err != nil {
			return nil, err
		}
		collector.inspectInputs(inputs)
	}

	out := []string{}
	for path := range collector.Collected {
		out = append(out, path)
	}

	return out, nil
}

// if this is a file object, collect the path
func (c *InputsCollector) collectIfFile(i interface{}) error {
	if isFile(i) {
		path, err := filePath(i)
		if err != nil {
			return err
		}
		c.Collected[path] = true
	}
	return nil
}

// inspects inputs.json and collects paths for any file parameters encountered
func (c *InputsCollector) inspectInputs(inputs map[string]interface{}) error {
	var err error
	for _, input := range inputs {
		switch reflVal := reflect.ValueOf(input); reflVal.Kind() {
		case reflect.Array:
			fmt.Println("handling reflected array")
			for _, i := range input.([]interface{}) {
				if err = c.collectIfFile(i); err != nil {
					fmt.Println("failed handling reflected array")
					return err
				}
			}
		default:
			if err = c.collectIfFile(input); err != nil {
				return err
			}
		}
	}
	return nil
}

// determines whether a map i represents a CWL file object
// lifted straight from the mariner package
// NOTE: there's gonna be some problems here, need to make changes in mariner code
func isFile(i interface{}) (f bool) {
	// here //
	if i == nil {
		return false
	}
	/////

	iType := reflect.TypeOf(i)
	fmt.Println("itType: ", iType)
	iKind := iType.Kind()
	if iKind == reflect.Map {
		iMap := reflect.ValueOf(i)
		for _, key := range iMap.MapKeys() {
			if key.Type() == reflect.TypeOf("") {
				if key.String() == "class" {
					// here //
					switch {
					case iMap.MapIndex(key).IsNil():
						// not a file (?)
						// double check this logic
					case iMap.MapIndex(key).Interface() == "File":
						f = true
					}
				}
			}
		}
	}
	return f
}

// get path from a file object which
// also from the mariner package
func filePath(i interface{}) (path string, err error) {
	iter := reflect.ValueOf(i).MapRange()
	for iter.Next() {
		key, val := iter.Key().String(), iter.Value()
		if key == "location" || key == "path" {
			return val.Interface().(string), nil
		}
	}
	return "", fmt.Errorf("no location or path specified")
}
