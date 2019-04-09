package gen3cwl

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type FakeEngine struct {
	TaskSequence []string
}

func (engine *FakeEngine) DispatchTask(jobID string, task *Task) error {
	// call k8s api to schedule job
	fmt.Printf("hey fake engine runs %v, %v \n", task.Parameters, task.Root.ID)
	switch task.Root.ID {
	case "#initdir_test.cwl":
		err := json.Unmarshal([]byte(`
			{"#initdir_test.cwl/bam_with_index": {
				"class": "File",
				"location": "NIST7035.1.chrM.bam",
				"secondaryFiles": [
					{
						"basename": "NIST7035.1.chrM.bam.bai",
						"location": "initdir_test.cwl/NIST7035.1.chrM.bam.bai",
						"class": "File"
					}
				]
			}}`), &task.Outputs)
		if err != nil {
			fmt.Printf("fail to unmarshal this thing\n")
		}
	case "#expressiontool_test.cwl":
		err := json.Unmarshal([]byte(`
			{"#expressiontool_test.cwl/output": [
				{"bam_with_index": {
					"class": "File",
					"location": "NIST7035.1.chrM.bam",
					"secondaryFiles": [
						{
							"basename": "NIST7035.1.chrM.bam.bai",
							"location": "initdir_test.cwl/NIST7035.1.chrM.bam.bai",
							"class": "File"
						}
					]
				}}
			]}`), &task.Outputs)
		if err != nil {
			fmt.Printf("fail to unmarshal this thing\n")
		}
	}
	engine.TaskSequence = append(engine.TaskSequence, task.Root.ID)
	return nil
}

func TestWorkflow(t *testing.T) {
	cwlfile, _ := os.Open("../testdata/gen3_test.pack.cwl")
	body, _ := ioutil.ReadAll(cwlfile)

	inputsfile, _ := os.Open("../testdata/inputs.json")
	inputs, _ := ioutil.ReadAll(inputsfile)
	engine := new(FakeEngine)
	err := RunWorkflow("123", body, inputs, engine)
	if err != nil {
		t.Error(err.Error())
	}
	fmt.Printf("steps: %v\n", engine.TaskSequence)
	assert.Equal(
		t,
		engine.TaskSequence,
		[]string{"#initdir_test.cwl", "#expressiontool_test.cwl", "#scatter_test.cwl"},
		"wrong task sequence",
	)
}
