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

	fmt.Print("\n")
	fmt.Print("finish")
	return nil
}

func resolve(workflow cwl.Root, status map[string]string, workflows map[string]cwl.Root) error {
	if _, ok := status[workflow.ID]; ok {
		// already run before
		return nil
	}
	if workflow.Class == "Workflow" {
		unfinishedSteps := make(map[string]cwl.Step)
		// store a map of outputID -> stepID to trace dependency
		dataMap := make(map[string]string)
		// create an unfinished steps map as a queue
		for _, step := range workflow.Steps {
			unfinishedSteps[step.ID] = step
			for _, output := range step.Out {
				dataMap[output.ID] = step.ID
			}
		}
		var curStepID string
		var prevStepID string
		var curStep cwl.Step
		//  pick a random first step
		for step := range unfinishedSteps {
			curStepID = step
			break
		}
		for len(unfinishedSteps) > 0 {

			// resolve dependent steps
			prevStepID = ""
			curStep = unfinishedSteps[curStepID]
			for _, input := range curStep.In {
				for _, source := range input.Source {
					// if source is an ID that points to an output in another step
					if stepID, ok := dataMap[source]; ok {
						if _, ok := unfinishedSteps[stepID]; ok {
							prevStepID = stepID
							break
						}
					}
				}
				if prevStepID != "" {
					break
				}
			}
			// cancel processing this step, go to next loop to process dependent step
			if prevStepID != "" {
				curStepID = prevStepID
				fmt.Printf("go to dependent step %v \n", curStepID)
				continue
			}
			//  run step
			fmt.Printf("run step %v \n", curStepID)
			if subworkflow, ok := workflows[curStep.Run.Value]; ok {
				resolve(subworkflow, status, workflows)
			} else {
				fmt.Print("submit single step to k8s ")
				fmt.Print(workflow.ID)
			}
			delete(unfinishedSteps, curStepID)
			// get random next step
			for step := range unfinishedSteps {
				curStepID = step
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
	// fmt.Print(workflow.Steps[0].ID)
	// fmt.Print(workflow.Steps[0].In[0].ValueFrom)
	// fmt.Print(workflow.Steps[0].In[0].Source)
	return nil
}
