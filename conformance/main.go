package main

func main() {

}

// path to config: 		./common-workflow-language/v1.0/conformance_test_v1.0.yaml
// path to test suite: 	./common-workflow-language/v1.0/v1.0/

const (
	config = "./common-workflow-language/v1.0/conformance_test_v1.0.yaml"
)

func runTests(apiKey string) error {
	tok, err := token(apiKey)
	if err != nil {
		return err
	}
	suite, err := loadConfig(config)
	if err != nil {
		return err
	}
	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		go runTest(test, tok)
	}
	return nil
}

func loadConfig(config string) ([]map[string]interface{}, error) {
	return nil, nil
}

func token(apiKey string) (string, error) {
	return "", nil
}

func runTest(test map[string]interface{}, tok string) {
	return
}
