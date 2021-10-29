package conformance

import (
	"fmt"
	"reflect"
	"strings"
)

// InputsCollector collects paths of input parameters of type File
type InputsCollector struct {
	Collected map[string]bool
}

// InputFiles ..
// given a test list
// return the list of files which must be staged
// -- to the target test environment
// -- in order for the tests to run as expected
func InputFiles(tests []*TestCase) ([]string, error) {
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
		out = append(out, strings.TrimPrefix(path, inputPathPrefix))
	}

	return out, nil
}

// if this is a file object, collect the path
func (c *InputsCollector) collectIfFile(i interface{}) error {
	if isClass(i, "File") {
		path, err := filePath(i)
		if err != nil {
			return err
		}
		c.Collected[path] = true
		if err = c.collectSecondary(i); err != nil {
			return err
		}
	}
	return nil
}

func (c *InputsCollector) collectSecondary(i interface{}) error {
	var path string
	var err error

	files := secondaryFiles(i)
	if files == nil {
		return nil
	}

	for _, f := range files {
		path, err = filePath(f)
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
// lifted from the mariner package
// TODO: need to make changes in mariner code
func isClass(i interface{}, class string) (f bool) {
	if i == nil {
		return false
	}

	iType := reflect.TypeOf(i)
	iKind := iType.Kind()
	if iKind == reflect.Map {
		iMap := reflect.ValueOf(i)
		for _, key := range iMap.MapKeys() {
			if key.Interface() == "class" {
				switch {
				case iMap.MapIndex(key).IsNil():
					return false
				case iMap.MapIndex(key).Interface() == class:
					f = true
				}
			}
		}
	}

	return f
}

// get path from a file object which
// also from the mariner package - todo: make corresponding changes to mariner code
func filePath(i interface{}) (path string, err error) {
	iter := reflect.ValueOf(i).MapRange()
	for iter.Next() {
		k, val := iter.Key(), iter.Value()
		key := k.Interface().(string)
		if key == "location" || key == "path" {
			return val.Interface().(string), nil
		}
	}
	return "", fmt.Errorf("no location or path specified")
}
