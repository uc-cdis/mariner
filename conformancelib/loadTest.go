package conformance

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/uc-cdis/mariner/wflib"
)

const (
	// all path/location of files need this prefix
	// in the user data s3 bucket, the directory structure is:
	// -- /userID/conformanceTesting/<file>
	inputPathPrefix = "USER/conformanceTesting/"
)

// UserDataSpaceLocation ..
// returns prefix appended to all input files/dirs
// this is the location (dir) in the user data space
// where all the input files/dirs will be staged
// i.e., where mariner will expect them to be,
// because all the paths get this path prefix affixed to them
func UserDataSpaceLocation() string {
	return inputPathPrefix
}

// affix the prefix
func processInputs(in map[string]interface{}) map[string]interface{} {
	for _, v := range in {
		if isClass(v, "File") || isClass(v, "Directory") {
			addPrefix(v)
			if sFiles := secondaryFiles(v); sFiles != nil {
				for _, sf := range sFiles {
					if isClass(sf, "File") || isClass(sf, "Directory") {
						addPrefix(sf)
					}
				}
			}
		}
	}
	return in
}

// this is painful to look at
// fixme: reflection can be used to fix this code
func addPrefix(f interface{}) {
	var path string
	var ok bool
	switch m := f.(type) {
	case map[string]interface{}:
		if path, ok = m["location"].(string); ok && path != "" {
			m["location"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
		}
		if path, ok = m["path"].(string); ok && path != "" {
			m["path"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
		}
	case map[interface{}]interface{}:
		if path, ok = m["location"].(string); ok && path != "" {
			m["location"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
		}
		if path, ok = m["path"].(string); ok && path != "" {
			m["path"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
		}
	}
}

// return list (or nil) of secondaryFiles for a given file object
func secondaryFiles(i interface{}) []interface{} {
	var files interface{}
	switch m := i.(type) {
	case map[string]interface{}:
		files = m["secondaryFiles"]
	case map[interface{}]interface{}:
		files = m["secondaryFiles"]
	}
	if files == nil {
		return nil
	}
	return files.([]interface{})
}

var inputFileExt = map[string]bool{
	".json": true,
	".yaml": true,
	".yml":  true,
}

// load inputs.json (or .yaml)
func (t *TestCase) input() (map[string]interface{}, error) {
	// fmt.Println("handling param set: ", t.Input)
	if t.Input == "" {
		return make(map[string]interface{}), nil
	}
	ext := filepath.Ext(t.Input)
	if !inputFileExt[ext] {
		return nil, fmt.Errorf("unexpected inputs file: %v ; testID: %v", t.Input, t.ID)
	}

	b, err := ioutil.ReadFile(t.Input)
	if err != nil {
		return nil, err
	}

	in := &map[string]interface{}{}
	switch ext {
	case ".json":
		err = json.Unmarshal(b, in)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(b, in)
	}
	if err != nil {
		return nil, err
	}

	input := processInputs(*in)

	return input, nil
}

// return tags to apply to test case workflow request
func (t *TestCase) tags() map[string]string {
	tags := make(map[string]string)
	tags["job"] = t.Input
	tags["tool"] = t.CWL
	tags["label"] = t.Label
	tags["id"] = string(t.ID)
	tags["doc"] = t.Doc
	tags["tags"] = strings.Join(t.Tags, ",")
	if t.ShouldFail {
		tags["should_fail"] = "true"
	} else {
		tags["should_fail"] = "false"
	}
	return tags
}

// load test case CWL as JSON
func (t *TestCase) workflow() (*wflib.WorkflowJSON, error) {
	wf, err := wflib.PackWorkflow(t.CWL)
	if err != nil {
		return nil, err
	}
	valid, grievances := wflib.ValidateWorkflow(wf)
	if !valid {
		return nil, fmt.Errorf("%v", grievances)
	}
	return wf, nil
}

// returns workflow request body as []byte
func wfBytes(wf *wflib.WorkflowJSON, in map[string]interface{}, tags map[string]string) ([]byte, error) {
	req := WorkflowRequest{
		Workflow: wf,
		Input:    in,
		Tags:     tags,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return b, nil
}
