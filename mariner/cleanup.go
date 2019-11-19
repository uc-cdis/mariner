package mariner

import (
	"time"

	cwl "github.com/uc-cdis/cwl.go"
)

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
// need to refactor and make nice
// also need to add logging for this process
func (task *Task) cleanupByStep() error {

	// 0. create the by-step map
	byStep := make(CleanupByStep)
	task.CleanupByStep = &byStep

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
			for _, otherStep := range task.Root.Steps {
				if otherStep.ID != step.ID {
					for _, input := range otherStep.In {
						// FIXME - assuming one source specified here - certainly require case handling
						// I *think* every input should have at least one source specified though
						if input.Source[0] == stepOutput.ID {
							deleteCondition.DependentSteps[otherStep.ID] = nil
							deleteCondition.Queue[otherStep.ID] = nil
							break
						}
					}
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

			// HERE:
			//
			// i) (TODO) update deleteCondition queue when a corresponding step finishes running
			//
			// i.5) (DONE) launch monitoring per step
			// ii) (DONE) delete action upon condition met

			// 6. monitor delete condition -> delete when condition == true
			go task.deleteFilesAtCondition(step, stepOutput.ID)
		}
	}
	return nil
}

// delete condition: !WorkflowOutput && len(Queue) == 0
// if the conditional will eventually be met, monitor the condition
// as soon as it evaluates to 'true' -> delete the files associated with this output parameter
//
// BIG NOTE: only need to monitor params of type FILE
// -------- FIXME - need to check param type, ensure that it is 'file', before monitoring/deleting
func (task *Task) deleteFilesAtCondition(step cwl.Step, outputParam string) {
	condition := (*task.CleanupByStep)[step.ID][outputParam]
	if !condition.WorkflowOutput {
		for {
			if len(condition.Queue) == 0 {
				task.deleteIntermediateFiles(step, outputParam)
			}
			// 30s is an arbitrary choice for initial development - can be optimized/changed moving forward
			time.Sleep(30 * time.Second)
		}
	}
}

// this function gets called iff
// 1. this step has finished running, and
// 2. all other steps of the parent workflow which use these files have all finished running
// 3. these files do not correspond to output params of the parent workflow
//
// i.e., the files are there and we know it's safe to delete them
func (task *Task) deleteIntermediateFiles(step cwl.Step, outputParam string) {
	childTask := task.Children[step.ID]
	subtaskOutputID := step2taskID(&task.Children[step.ID].OriginalStep, outputParam)
	fileOutput := childTask.Outputs[subtaskOutputID]
	// 'files' is either type *File or []*File
	var err error
	switch fileOutput.(type) {
	case *File:
		f := fileOutput.(*File)
		err = f.delete()
		if err != nil {
			// log
		}
	case []*File:
		files := fileOutput.([]*File)
		for _, f := range files {
			err := f.delete()
			if err != nil {
				// log; attempt delete on all files, even if some fail
			}
		}
	}
}
