package gen3cwl

import (
	"encoding/json"
	"fmt"

	cwl "github.com/uc-cdis/cwl.go"
)

// RunWorkflow parses a workflow and inputs and run it
func RunWorkflow(workflow []byte) error {
	var root cwl.Root
	err := json.Unmarshal(workflow, &root)
	if err != nil {
		return ParseError(err)
	}
	workflows := make(map[string]cwl.Root)
	status := make(map[string]string)

	for _, workflow := range root.Graphs {
		workflows[workflow.ID] = *workflow
	}
	resolve(*root.Graphs[0], status, workflows)

	return nil
}

func resolve(workflow cwl.Root, status map[string]string, workflows map[string]cwl.Root) error {
	if _, ok := status[workflow.ID]; ok {
		// already run before
		return nil
	}
	fmt.Print("run workflow")
	fmt.Print(workflow.ID)
	fmt.Print("\n")
	limit := 0
	if workflow.Class == "Workflow" {
		unfinishedSteps := make(map[string]*cwl.Step)
		// store a map of outputID -> stepID to trace dependency
		dataMap := make(map[string]string)
		// create an unfinished steps map as a queue
		for _, step := range workflow.Steps {
			unfinishedSteps[step.ID] = &step
			for _, output := range step.Out {
				dataMap[output.ID] = step.ID
			}
		}
		var curStep cwl.Step
		var prevStep *cwl.Step
		//  pick a random first step
		for step := range unfinishedSteps {
			curStep = *unfinishedSteps[step]
			break
		}
		for len(unfinishedSteps) > 0 {
			if limit > 100 {
				break
			}
			limit = limit + 1
			fmt.Printf("getlimit %v \n", limit)
			// resolve dependent steps
			prevStep = nil
			for _, input := range curStep.In {
				for _, source := range input.Source {
					// if source is an ID that points to an output in another step
					if dependentStep, ok := dataMap[source]; ok {
						prevStep = unfinishedSteps[dependentStep]
						break
					}
				}
				if prevStep != nil {
					break
				}
			}
			// cancel processing this step, go to next loop to process dependent step
			if prevStep != nil {
				curStep = *prevStep
				fmt.Printf("go to dependent step %v \n", curStep.ID)
				continue
			}
			//  run step
			fmt.Printf("run step %v \n", curStep.ID)
			if subworkflow, ok := workflows[curStep.Run.Value]; ok {
				resolve(subworkflow, status, workflows)
			} else {
				fmt.Print("submit single step to k8s ")
				fmt.Print(workflow.ID)
			}
			fmt.Printf("delete step %v \n", curStep.ID)
			delete(unfinishedSteps, curStep.ID)
			fmt.Printf("map length %v \n", len(unfinishedSteps))
			// get random next step
			for step := range unfinishedSteps {
				fmt.Printf("loop %v \n", step)
				curStep = *unfinishedSteps[step]
				break
			}
		}
	} else {
		fmt.Print("submit single job to k8s ")
		fmt.Print(workflow.Class)
		fmt.Print(workflow.ID)
		status[workflow.ID] = "success"
		fmt.Print("\n")
	}
	return nil
}
