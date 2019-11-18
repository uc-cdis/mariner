package mariner

import "time"

// CleanupByStep exists per workflow
// it's a collection of (stepID, CleanupByParam)
type CleanupByStep map[string]CleanupByParam

// CleanupByParam exists per workflow step
// it's a collection of (ouputParamID, CleanupFlags) pairs
type CleanupByParam map[string]*DeleteCondition

// DeleteCondition exists per output parameter
// we want to delete all intermediate files
// as soon as they become unnecessary to downstream processes
// the condition for deletion is: !WorkflowOutput && len(Queue) == 0
type DeleteCondition struct {
	WorkflowOutput bool                   // this an output of the top-level workflow
	DependentSteps map[string]interface{} // static collection of steps which depend on using this output parameter
	Queue          map[string]interface{} // each time a dependent step finishes, remove it from the Queue
}

// dev'ing this feature
func (task *Task) setupCleanupByStep() error {

	// 0. create the by-step map
	task.CleanupByStep = new(CleanupByStep)

	// 1. then, for each step
	for _, step := range task.Root.Steps {

		// 2. create the by-param map
		(*task.CleanupByStep)[step.ID] = make(CleanupByParam)

		// 3. then, for each output param
		for _, stepOutput := range step.Out {

			// 4. create the zero-value delete condition struct
			(*task.CleanupByStep)[step.ID][stepOutput.ID] = &DeleteCondition{
				WorkflowOutput: false,
				DependentSteps: make(map[string]interface{}),
				Queue:          make(map[string]interface{}),
			}
			deleteCondition := (*task.CleanupByStep)[step.ID][stepOutput.ID]

			// 5. then, populate the delete condition struct:

			// 5A. collect the IDs of all the other steps of this workflow
			// --- which will use files associated with this output param
			for _, input := range step.In {
				// FIXME - assuming one source specified here - certainly require case handling
				// I *think* every input should have at least one source specified though
				if input.Source[0] == stepOutput.ID {
					deleteCondition.DependentSteps[input.ID] = nil
					deleteCondition.Queue[input.ID] = nil
				}
			}

			// 5B. determine whether this output param is a workflow output param
			for _, workflowOutput := range task.Root.Outputs {
				// FIXME - again assuming exactly one source - need case handling
				// also need to determine whether it should ever be the case that len(source) != 1
				if workflowOutput.Source[0] == stepOutput.ID {
					deleteCondition.WorkflowOutput = true
					break
				}
			}

			//// --- ////

			// HERE - probably need to restructure this
			// TODO:
			// i) update deleteCondition queue when a corresponding step finishes running
			// ii) delete action upon condition met

			// 6. monitor delete condition -> delete when condition == true
			go deleteCondition.monitor()
		}
	}
	return nil
}

// delete condition: !WorkflowOutput && len(Queue) == 0
// if the conditional will eventually be met, monitor the condition
// as soon as it evaluates to 'true' -> delete the files associated with this output parameter
func (deleteCondition *DeleteCondition) monitor() {
	if !deleteCondition.WorkflowOutput {
		for {
			if len(deleteCondition.Queue) == 0 {
				// delete files! somehow
				// just need to access the [files]
				// which is the val to this output param key
				// in task.Outputs map
			}

			// refresh every 30s
			// (30s is an arbitrary choice - probably there's a better refresh-window)
			time.Sleep(30 * time.Second)
		}
	}
}
