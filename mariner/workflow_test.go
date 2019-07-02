package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

type FakeEngine struct {
	TaskSequence []string
	Commands     map[string][]string
}

// this is what task.Outputs should look like!
func (engine *FakeEngine) DispatchTask(jobID string, task *Task) error {
	// call k8s api to schedule job
	fmt.Printf("\n---\n---\n")
	fmt.Printf("Dispatching task: %v\n\n", task.Root.ID)
	// fmt.Printf("\n--- hey fake engine runs %v, %v ---\n\n", task.Parameters, task.Root.ID)
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
	// check if task is commandlinetool or expressiontool
	fmt.Printf("\nClass: %v\n", task.Root.Class)
	switch class := task.Root.Class; class {
	case "CommandLineTool":
		tool := &Tool{
			Root:       task.Root,
			Parameters: task.Parameters,
		}
		tool.GenerateCommand()
		engine.Commands[task.Root.ID] = tool.Command.Args
	case "ExpressionTool":
		fmt.Printf("\nThis is an ExpressionTool: %v\n", task.Root.ID)
		fmt.Printf("\nNeed to handle this expression tool\n")
	}
	return nil
}

func TestWorkflow(t *testing.T) {
	cwlfile, _ := os.Open("../testdata/gen3_test.pack.cwl")
	body, _ := ioutil.ReadAll(cwlfile)

	inputsfile, _ := os.Open("../testdata/inputs.json")
	inputs, _ := ioutil.ReadAll(inputsfile)
	// engine := new(FakeEngine)
	engine := new(K8sEngine)
	engine.Commands = make(map[string][]string)
	engine.FinishedProcs = make(map[string]*Process)
	engine.UnfinishedProcs = make(map[string]*Process)
	err := RunWorkflow("123", body, inputs, engine)
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
