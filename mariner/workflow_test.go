package mariner

import (
	"io/ioutil"
	"os"
	"testing"
)

// currently doesn't run
// TODO - write tests, allow for testing locally in some capacity
func NotTestWorkflow(t *testing.T) {
	cwlfile, _ := os.Open("../testdata/gen3_test.pack.cwl")
	body, _ := ioutil.ReadAll(cwlfile)

	inputsfile, _ := os.Open("../testdata/inputs.json")
	inputs, _ := ioutil.ReadAll(inputsfile)
	engine := new(K8sEngine)
	engine.Commands = make(map[string][]string)
	engine.FinishedProcs = make(map[string]*Process)
	engine.UnfinishedProcs = make(map[string]*Process)
	err := runWorkflow(body, inputs, engine)
	if err != nil {
		t.Error(err.Error())
	}
	/*
		fmt.Printf("\nStep Order: %v\n\n", engine.TaskSequence)
		fmt.Printf("\nCommands:\n")
		for id, cmd := range engine.Commands {
			fmt.Printf("\n%v: %v\n", id, cmd)
		}
	*/
	/*
		assert.Equal(
			t,
			engine.TaskSequence,
			[]string{"#initdir_test.cwl", "#expressiontool_test.cwl", "#scatter_test.cwl"},
			"wrong task sequence",
		)
	*/
}
