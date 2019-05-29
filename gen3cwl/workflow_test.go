package gen3cwl

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	cwl "github.com/uc-cdis/cwl.go"
)

type FakeEngine struct {
	TaskSequence []string
}

func (engine *FakeEngine) DispatchTask(jobID string, task *Task) error {
	// call k8s api to schedule job
	fmt.Printf("hey fake engine runs %v, %v \n", task.Parameters, task.Root.ID)

	engine.TaskSequence = append(engine.TaskSequence, task.Root.ID)
	return nil
}
func (engine *FakeEngine) ListenOutputs(jobID string, task *Task) chan TaskResult {

	resultCh := make(chan TaskResult)
	go func() {
		result := TaskResult{}
		outputs := make(cwl.Parameters)
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
				}}`), &outputs)
			if err != nil {
				result.Error = err
				resultCh <- result
			} else {
				result.Outputs = outputs
				resultCh <- result
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
				result.Error = err
				resultCh <- result
			} else {
				result.Outputs = outputs
				resultCh <- result
			}
		}
		if result.Outputs == nil && result.Error == nil {
			result.Error = fmt.Errorf("problem getting output")
			resultCh <- result
		}

	}()

	return resultCh
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
	// assert.Equal(
	// 	t,
	// 	engine.TaskSequence,
	// 	[]string{"#initdir_test.cwl", "#expressiontool_test.cwl", "#scatter_test.cwl"},
	// 	"wrong task sequence",
	// )
}
