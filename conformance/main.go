package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/yaml.v2"

	// wftool "github.com/uc-cdis/mariner/wftool"
)

// TestCase ..
type TestCase struct {
	Input      string                 `json:"job"`         // path to input.json (may also be yaml)
	Output     map[string]interface{} `json:"output"`      // expected output
	ShouldFail bool                   `json:"should_fail"` // if the engine is expected to fail on this cwl
	CWL        string                 `json:"tool"`        // path to tool.cwl
	Label      string                 `json:"label"`
	ID         int                    `json:"id"`
	Doc        string                 `json:"doc"`
	Tags       []string               `json:"tags"`
}

// Runner ..
type Runner struct {
	Token *AccessToken
}

func main() {

}

// path to config: 		./common-workflow-language/v1.0/conformance_test_v1.0.yaml
// path to test suite: 	./common-workflow-language/v1.0/v1.0/

const (
	// path to tests config - don't hardcode (?)
	config = "./common-workflow-language/v1.0/conformance_test_v1.0.yaml"
	// of course, avoid hardcoding
	// could pass commons url as param
	tokenEndpoint = "https://mattgarvin1.planx-pla.net/user/credentials/api/access_token"
)

// 'creds' is path/to/creds.json which is what you get
// when you create and download an apiKey from the portal
func runTests(creds string) error {
	suite, err := loadConfig(config)
	if err != nil {
		return err
	}
	tok, err := token(creds)
	if err != nil {
		return err
	}
	runner := Runner{Token: tok}
	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		// go runTest(test, tok) // todo
		runner.Run(test) // dev with sequential, then make concurrent
	}
	return nil
}

func loadConfig(config string) ([]TestCase, error) {
	b, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	testSuite := new([]TestCase)
	if err = yaml.Unmarshal(b, testSuite); err != nil {
		return nil, err
	}
	return *testSuite, nil
}

// Creds is creds.json, as downloaded from the portal
type Creds struct {
	APIKey string `json:"api_key"`
	KeyID  string `json:"key_id"`
}

// AccessToken response from fence /access_token endpoint
type AccessToken struct {
	Token string `json:"access_token"`
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

func token(creds string) (*AccessToken, error) {
	body, err := apiKey(creds)
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(tokenEndpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	accessToken := &AccessToken{}
	err = json.Unmarshal(b, accessToken)
	if err != nil {
		return nil, err
	}
	return accessToken, nil
}

// Run ..
// here - todo
func (r *Runner) Run(test TestCase) {

	fmt.Println(test)

	// todo - make a type to match config test struct
	// then these other functions can be methods of that struct
	/*
		// 1. pack the CWL to json (wf)
		wf, err := wftool.PackWorkflow(test["tool"])
		valid, grievances := wftool.ValidateWorkflow(wf)
		if !valid {
			error()
		}

		// 2. load inputs
		input, err := loadInputs(test["job"])

		// 3. make request body -> use type from mariner server
		body, err := {
			wf,
			input,
		}

		// 4. pass request to mariner
		// 5. listen for done (or err/fail)
		// 6. match output
		// 7. save/record result

	*/
	return
}

// printJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
