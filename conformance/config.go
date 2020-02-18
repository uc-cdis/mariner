package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// path to config: 		./common-workflow-language/v1.0/conformance_test_v1.0.yaml
// path to test suite: 	./common-workflow-language/v1.0/v1.0/

// fixme: do NOT hardcode
const (
	// path to tests config - don't hardcode (?)
	config = "./common-workflow-language/v1.0/conformance_test_v1.0.yaml"

	// directory containing all the cwl files and input json/yamls
	// github.com/uc-cdis/mariner/conformance/common-workflow-language/v1.0/v1.0
	pathToTests = "./common-workflow-language/v1.0/v1.0/"

	// all path/location of files need this prefix
	// in the user data s3 bucket, the directory structure is:
	// -- /userID/conformanceTesting/<file>
	inputPathPrefix = "USER/conformanceTesting/"
)

// TestCase ..
type TestCase struct {
	Input      string                 `json:"job" yaml:"job"`                 // path to input.json (may also be yaml)
	Output     map[string]interface{} `json:"output" yaml:"output"`           // expected output
	ShouldFail bool                   `json:"should_fail" yaml:"should_fail"` // if the engine is expected to fail on this cwl
	CWL        string                 `json:"tool" yaml:"tool"`               // path to tool.cwl
	Label      string                 `json:"label" yaml:"label"`
	ID         int                    `json:"id" yaml:"id"`
	Doc        string                 `json:"doc" yaml:"doc"`
	Tags       []string               `json:"tags" yaml:"tags"`
}

func loadConfig(config string) ([]*TestCase, error) {
	b, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	testSuite := new([]*TestCase)
	if err = yaml.Unmarshal(b, testSuite); err != nil {
		return nil, err
	}
	for _, t := range *testSuite {
		if t.Input != "" {
			t.Input = fmt.Sprintf("%v%v", pathToTests, filepath.Base(t.Input))
		}
		if t.CWL != "" {
			t.CWL = fmt.Sprintf("%v%v", pathToTests, filepath.Base(t.CWL))
		}
	}

	return *testSuite, nil
}
