package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/yaml.v2"
)

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

	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		go runTest(test, tok) // todo
	}
	return nil
}

func loadConfig(config string) ([]map[string]interface{}, error) {
	b, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, err
	}
	i := new(interface{})
	err = yaml.Unmarshal(b, i)
	if err != nil {
		return nil, err
	}
	testSuite, ok := (*i).([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected config structure")
	}
	return testSuite, nil
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

// here - todo
func runTest(test map[string]interface{}, tok *AccessToken) {
	return
}
