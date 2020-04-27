package conformance

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// path to config: 		./common-workflow-language/v1.0/conformance_test_v1.0.yaml
// path to test suite: 	./common-workflow-language/v1.0/v1.0/

// fixme: do NOT hardcode
const (
	// path to tests config relative to common-workflow-language dir
	testsConfig = "/v1.0/conformance_test_v1.0.yaml"

	defaultPathToCWLDir = "./common-workflow-language"

	// directory containing all the cwl files and input json/yamls
	// github.com/uc-cdis/mariner/conformance/common-workflow-language/v1.0/v1.0
	testsDir = "/v1.0/v1.0/"
)

// TestCase ..
type TestCase struct {
	Input      string      `json:"job" yaml:"job"`                 // path to input.json (may also be yaml)
	Output     interface{} `json:"output" yaml:"output"`           // expected output
	ShouldFail bool        `json:"should_fail" yaml:"should_fail"` // if the engine is expected to fail on this cwl
	CWL        string      `json:"tool" yaml:"tool"`               // path to tool.cwl
	Label      string      `json:"label" yaml:"label"`
	ID         int         `json:"id" yaml:"id"`
	Doc        string      `json:"doc" yaml:"doc"`
	Tags       []string    `json:"tags" yaml:"tags"`
}

// Creds is creds.json, as downloaded from the portal
type Creds struct {
	APIKey string `json:"api_key"`
	KeyID  string `json:"key_id"`
}

func loadConfig(pathToCWLDir string) ([]*TestCase, error) {
	if pathToCWLDir == "" {
		pathToCWLDir = defaultPathToCWLDir
	}
	config := filepath.Join(pathToCWLDir, testsConfig)
	b, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	testSuite := new([]*TestCase)
	if err = yaml.Unmarshal(b, testSuite); err != nil {
		return nil, err
	}
	pathToTests := filepath.Join(pathToCWLDir, testsDir)
	for _, t := range *testSuite {
		if t.Input != "" {
			t.Input = filepath.Join(pathToTests, filepath.Base(t.Input))
		}
		if t.CWL != "" {
			t.CWL = filepath.Join(pathToTests, filepath.Base(t.CWL))
		}
	}

	return *testSuite, nil
}

// read creds into Creds struct
func apiKey(creds string) ([]byte, error) {
	// read in bytes
	b, err := ioutil.ReadFile(creds)
	if err != nil {
		return nil, err
	}

	// validate against creds schema
	c := &Creds{}
	err = json.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}
	if c.APIKey == "" || c.KeyID == "" {
		return nil, fmt.Errorf("missing credentials")
	}

	// return bytes
	return b, nil
}
