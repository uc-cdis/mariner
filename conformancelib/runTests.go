package main

import "fmt"

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

	r := Runner{
		Token:   tok,
		Results: new(Results),
	}
	r.Results.Error = make(map[int]error)

	for _, test := range suite {
		// could make a channel to capture errors from individual tests
		// go runTest(test, tok)

		// dev with sequential, then make concurrent (?)
		if err = r.Run(test); err != nil {
			fmt.Println("err running test: ", err)
			r.Results.Error[test.ID] = err
		}
	}

	// for now
	/*
		fmt.Println("here are the results:")
		printJSON(r.Results)
	*/

	fmt.Println("fin")

	return nil
}
