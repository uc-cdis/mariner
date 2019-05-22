package gen3cwl

import (
	"fmt"

	cwl "github.com/uc-cdis/cwl.go"
)

// Engine defines specific implementation for an engine
type Engine interface {
	DispatchTask(jobID string, task *Task) error
}

// K8sEngine uses k8s Job API to run workflows
type K8sEngine struct {
	TaskSequence []string
	Commands     map[string][]string // for testing purposes
}

// DispatchTask runs the tool as a docker container
// the engine needs to populate task.Outputs with the output after the task has been executed (?)
func (engine K8sEngine) DispatchTask(jobID string, task *Task) error {
	// call k8s api to schedule job
	/*
		// temporarily hardcoding output here..
		// need to write code to collect/generate this per task
		// task.Root.Outputs is already populated from the cwl
		// task.Outputs needs to be populated, probably using task.Root.Outputs plus output bindings
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
	*/

	if task.Root.ID == "#scatter_test.cwl" {
		fmt.Println("\tNOT dispatching task #scatter_test.cwl")
		fmt.Println("\tStill need to implement scatter functionality.")
		return nil
	}
	switch class := task.Root.Class; class {
	case "CommandLineTool":
		fmt.Println("\tThis is a command line tool.")
		clt := &CommandLineTool{
			Root:       task.Root,
			Parameters: task.Parameters,
		}
		fmt.Println("\tGenerating command..")
		clt.GenerateCommand()
		fmt.Println("\tRunning k8s job..")
		err := clt.RunK8sJob()
		if err != nil {
			fmt.Printf("\tERROR: %v\n", err)
			return err
		}
		// here we need to generate the output parameter for this task
		// and store it in task.Outputs

		fmt.Println("\tGathering outputs..")
		clt.GatherOutputs() // not totally sure what happens here

		// construct the task output parameters here
		/*
			task.Outputs = clt.ConstructOutputs()
			fmt.Println("Task Outputs:")
			PrintJSON(task.Outputs)
		*/

	case "ExpressionTool":
		fmt.Println("\tThis is an ExpressionTool.")
		fmt.Println("\tExpression tools presently not supported.")
		exp := &ExpressionTool{
			Root:       task.Root,
			Parameters: task.Parameters,
			Expression: task.Root.Expression,
			Outputs:    make(cwl.Parameters),
		}
		err := exp.RunExpressionTool()
		if err != nil {
			fmt.Println("\tERROR: Failed to run ExpressionTool.")
			return err
		}
		task.Outputs = exp.Outputs
	}
	return nil
}
