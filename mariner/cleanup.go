package mariner

import (
	"fmt"
	"os"
	"sync"
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
	WorkflowOutput bool            // this an output of the top-level workflow
	DependentSteps map[string]bool // static collection of steps which depend on using this output parameter
	Queue          *GoQueue        // each time a dependent step finishes, remove it from the Queue
}

// TODO - create separate concurrent-safe map types for different kinds of maps
// there are only a few

// GoQueue is safe for concurrent read/write
type GoQueue struct {
	sync.RWMutex
	Map map[string]bool
}

func (m *GoQueue) update(k string, v bool) {
	m.Lock()
	defer m.Unlock()
	m.Map[k] = v
}

func (m *GoQueue) delete(k string) {
	m.Lock()
	defer m.Unlock()
	delete(m.Map, k)
}

// CleanupKey uniquely identifies (within a workflow) a set of files to monitor/delete
type CleanupKey struct {
	StepID string
	Param  string
}

// NOTE: only want to keep track of files - not any other kind of param
// ----- the type information is not contained in the step-level params
// ----- so if you want to check a param type, you must access the task object and check that param within the task struct

// super straightforward - write a function: f(stepParam) -> paramType (in cwl terms)

// dev'ing this feature
// need to refactor and make nice
// also need to add logging for this process
func (engine *K8sEngine) cleanupByStep(task *Task) error {

	fmt.Println("\tin cleanupByStep..")

	// 0. create the by-step map
	byStep := make(CleanupByStep)
	task.CleanupByStep = &byStep

	// flag indicates whether param correspons to a file(s)
	var fileParam = false

	// 1. then, for each step
	for _, step := range task.Root.Steps {

		fmt.Println("\tprocessing step: ", step.ID)
		// 2. create the by-param map
		(*task.CleanupByStep)[step.ID] = make(CleanupByParam)

		// 3. then, for each output param
		for _, stepOutput := range step.Out {
			fileParam = false

			fmt.Println("\tprocessing output param: ", stepOutput.ID)
			// 4. create the zero-value delete condition struct
			(*task.CleanupByStep)[step.ID][stepOutput.ID] = &DeleteCondition{
				WorkflowOutput: false,
				DependentSteps: make(map[string]bool),
				Queue: &GoQueue{
					Map: make(map[string]bool),
				},
			}

			deleteCondition := (*task.CleanupByStep)[step.ID][stepOutput.ID]

			// 5. then, populate the delete condition struct:

			// 5A. collect the IDs of all the other steps of this workflow
			// --- which will use files associated with this output param
			for _, otherStep := range task.Root.Steps {
				if otherStep.ID != step.ID {
					fmt.Println("\tlooking at other step: ", otherStep.ID)
					for _, input := range otherStep.In {
						fmt.Println("\tlooking at input pararm: ", input.ID)
						// FIXME - assuming one source specified here - certainly require case handling
						// I *think* every input should have at least one source specified though
						// HERE - check that param corresponds to files
						if input.Source[0] == stepOutput.ID && task.stepParamFile(&otherStep, input.ID) {
							fmt.Println("\tfound step dependency!")

							deleteCondition.DependentSteps[otherStep.ID] = true
							deleteCondition.Queue.update(otherStep.ID, true)

							fileParam = true

							break
						}
					}
				}
			}

			// 5B. determine whether this output param is a workflow output param
			fmt.Println("\tchecking if workflow output..")
			for _, workflowOutput := range task.Root.Outputs {
				fmt.Println("\tlooking at workflow output: ", workflowOutput.ID)
				// FIXME - again assuming exactly one source - need case handling
				// also need to determine whether it should ever be the case that len(source) != 1
				// HERE - check that param corresponds to files
				if workflowOutput.Source[0] == stepOutput.ID && outputParamFile(workflowOutput) {
					fmt.Println("\tfound parent dependency!")

					fmt.Println("workflowOutput:")
					printJSON(workflowOutput)

					deleteCondition.WorkflowOutput = true

					fileParam = true

					break
				}
			}

			// 6. monitor delete condition -> delete when condition == true
			// only launch these routines if param corresponds to files
			if fileParam {
				fmt.Println("\tlaunching go routine to delete files at condition")
				key := CleanupKey{step.ID, stepOutput.ID}
				engine.CleanupProcs[key] = true
				go engine.monitorParamDeps(task, step.ID, stepOutput.ID)
				go engine.deleteFilesAtCondition(task, step, stepOutput.ID)
			}
		}
	}
	return nil
}

// 'task' is a workflow
// monitors status of steps depending on files corresponding to the given output param of the given step; updates param queue appropriately
func (engine *K8sEngine) monitorParamDeps(task *Task, stepID string, param string) {
	condition := (*task.CleanupByStep)[stepID][param]
	for depStepID := range condition.DependentSteps {
		go func(task *Task, depStepID string, condition *DeleteCondition) {
			// wait for depTask to finish
			for !(*task.Children[depStepID].Done) {
			}
			// now depTask is done running - remove it from this param's dep queue
			condition.Queue.delete(depStepID)
		}(task, depStepID, condition)
	}
}

// delete condition: !WorkflowOutput && len(Queue) == 0
// if the conditional will eventually be met, monitor the condition
// as soon as it evaluates to 'true' -> delete the files associated with this output parameter
//
// BIG NOTE: only need to monitor params of type FILE
// -------- FIXME - need to check param type, ensure that it is 'file', before monitoring/deleting
func (engine *K8sEngine) deleteFilesAtCondition(task *Task, step cwl.Step, outputParam string) {
	fmt.Println("\tin deleteFilesAtCondition for: ", step.ID, outputParam)
	condition := (*task.CleanupByStep)[step.ID][outputParam]
	if !condition.WorkflowOutput {
		fmt.Println("\tnot parent workflow outputs; waiting to delete files: ", step.ID, outputParam)
		for {
			fmt.Println("\tlength of queue: ", len(condition.Queue.Map), step.ID, outputParam)
			printJSON(condition.Queue.Map)
			if len(condition.Queue.Map) == 0 {
				fmt.Println("\tdelete condition met! deleting files..")
				engine.deleteIntermediateFiles(task, step, outputParam)
				return
			}
			time.Sleep(15 * time.Second)
		}
	}
	fmt.Println("\tnot deleting files because parent workflow dependency: ", step.ID, outputParam)
	fmt.Println("\tupdating cleanupProc stack..")
	delete(engine.CleanupProcs, CleanupKey{step.ID, outputParam}) // maybe just put this in one place, not have it twice
}

// this function gets called iff
// 1. this step has finished running, and
// 2. all other steps of the parent workflow which use these files have all finished running
// 3. these files do not correspond to output params of the parent workflow
//
// i.e., the files are there and we know it's safe to delete them
//
// Q: what about secondaryFiles? probably those should be deleted as well
// -- assuming an intermediate file's secondaryFiles are also "intermediate" and ultimately not needed
func (engine *K8sEngine) deleteIntermediateFiles(task *Task, step cwl.Step, outputParam string) {
	fmt.Println("\tin deleteIntermediateFiles for: ", step.ID, outputParam)
	childTask := task.Children[step.ID]
	subtaskOutputID := step2taskID(&task.Children[step.ID].OriginalStep, outputParam)
	fileOutput := childTask.Outputs[subtaskOutputID]
	fmt.Println("\there is fileOutput:")
	printJSON(fileOutput)
	fmt.Printf("\t(%T)\n", fileOutput)
	// 'files' is either type *File or []*File
	var err error
	switch fileOutput.(type) {
	case *File:
		f := fileOutput.(*File)
		fmt.Println("\tdeleting single file: ", f.Location)
		err = f.delete()
		if err != nil {
			fmt.Println("failed to delete single file: ", err)
			// log
		}
	// NOTE: an array of files had this type: '[]map[string]interface{}' - why? in any case, need to iterate over this and delete each file
	// works for a leaf in the graph, not a workflow
	case []*File:
		fmt.Println("\tdeleting array of files..")
		files := fileOutput.([]*File)
		for _, f := range files {
			fmt.Println("\tdeleting file: ", f.Location)
			err := f.delete()
			if err != nil {
				fmt.Println("\tfailed to delete file: ", f.Location, err)
				// log; attempt delete on all files, even if some fail
			}
		}
	// *maybe in general* catches workflow output of file array (?)
	case []map[string]interface{}:
		files := fileOutput.([]map[string]interface{})
		var path string
		for _, m := range files {
			path, err = filePath(m)
			if err != nil {
				fmt.Println("error extracting path from 'file' in array:")
				printJSON(m)
				continue
			}
			err = os.Remove(path)
			if err != nil {
				fmt.Println("error deleting file: ", err)
			}
		}

	}
	fmt.Println("\tfinished deleting files, updating cleanupProc stack..")
	delete(engine.CleanupProcs, CleanupKey{step.ID, outputParam})
}
