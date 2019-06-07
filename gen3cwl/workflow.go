package gen3cwl

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	cwl "github.com/uc-cdis/cwl.go"
)

// Task defines an instance of workflow/tool
// a task is a process is a node on the graph is one of [Workflow, CommandLineTool, ExpressionTool, ...]
type Task struct {
	// Engine          Engine
	Engine          *K8sEngine
	JobID           string
	Parameters      cwl.Parameters
	Root            *cwl.Root
	Outputs         cwl.Parameters
	Scatter         []string
	ScatterMethod   string
	ScatterTasks    map[int]*Task
	ScatterIndex    int // if a task gets scattered, each subtask belonging to that task gets enumerated, and that index is stored here
	Children        map[string]*Task
	unFinishedSteps map[string]struct{}
	outputIDMap     map[string]string
	originalStep    cwl.Step
}

func resolveGraph(rootMap map[string]*cwl.Root, curTask *Task) error {
	if curTask.Root.Class == "Workflow" {
		curTask.Children = make(map[string]*Task)
		for _, step := range curTask.Root.Steps {
			subworkflow, ok := rootMap[step.Run.Value]
			if !ok {
				panic(fmt.Sprintf("can't find workflow %v", step.Run.Value))
			}
			newTask := &Task{
				JobID:        curTask.JobID,
				Engine:       curTask.Engine,
				Root:         subworkflow,
				Parameters:   make(cwl.Parameters),
				originalStep: step,
			}
			resolveGraph(rootMap, newTask)
			// what to use as id? value or step.id
			curTask.Children[step.ID] = newTask
		}
	}
	return nil
}

// RunWorkflow parses a workflow and inputs and run it
func RunWorkflow(jobID string, workflow []byte, inputs []byte, engine *K8sEngine) error {
	var root cwl.Root
	err := json.Unmarshal(workflow, &root)
	if err != nil {
		return ParseError(err)
	}

	var originalParams cwl.Parameters
	err = json.Unmarshal(inputs, &originalParams)

	var params = make(cwl.Parameters)
	for id, value := range originalParams {
		params["#main/"+id] = value
	}
	if err != nil {
		return ParseError(err)
	}
	var mainTask *Task
	flatRoots := make(map[string]*cwl.Root)

	// iterate through master list of all process objects in packed cwl json
	for _, workflow := range root.Graphs {
		flatRoots[workflow.ID] = workflow // populate process.ID: process pair
		if workflow.ID == "#main" {
			mainTask = &Task{JobID: jobID, Root: workflow, Parameters: params, Engine: engine} // construct mainTask (task object for the top level workflow)
		}
	}
	if mainTask == nil {
		panic(fmt.Sprint("can't find main workflow"))
	}

	resolveGraph(flatRoots, mainTask)
	mainTask.Run()

	fmt.Print("\n\nFinished running workflow job.\n")
	fmt.Println("Here's the output:")
	PrintJSON(mainTask.Outputs)
	return nil
}

func (task *Task) setupStepQueue() error {
	task.unFinishedSteps = make(map[string]struct{})
	for _, step := range task.Root.Steps {
		task.unFinishedSteps[step.ID] = struct{}{}
	}
	return nil
}

func (task *Task) getStep() string {
	for i := range task.unFinishedSteps {
		return i
	}
	return ""
}

// "#expressiontool_test.cwl" + "[#subworkflow_test.cwl]/test_expr/file_array"
// returns "#expressiontool_test.cwl/test_expr/file_array"
func step2taskID(step *cwl.Step, stepVarID string) string {
	return step.Run.Value + strings.TrimPrefix(stepVarID, step.ID)
}

// mergeChildOutputs maps outputs from children tasks to this task
// i.e., task.Outputs is a map of (outputID, outputValue) pairs
// for all the outputs of this workflow (this task is necessarily a workflow since only workflows have steps/children/subtasks)
func (task *Task) mergeChildOutputs() error {
	task.Outputs = make(cwl.Parameters)
	if task.Children == nil {
		panic(fmt.Sprintf("Can't call merge child outputs without childs %v \n", task.Root.ID))
	}
	for _, output := range task.Root.Outputs {
		/* example of workflow outputs from test cwl
		"outputs": [
				{
						"outputSource": "#subworkflow_test.cwl/test_expr/output",
						"type": {
								"items": "File",
								"type": "array"
						},
						"id": "#subworkflow_test.cwl/output_files"
				}
		]
		*/
		if len(output.Source) == 1 {
			source := output.Source[0]
			stepID, ok := task.outputIDMap[source]
			if !ok {
				panic(fmt.Sprintf("Can't find output source %v", source))
			}
			subtaskOutputID := step2taskID(&task.Children[stepID].originalStep, source)
			outputVal, ok := task.Children[stepID].Outputs[subtaskOutputID]
			if !ok {
				fmt.Printf("\tFail to get output from child step %v, %v\n\n", source, stepID)
			}
			task.Outputs[output.ID] = outputVal
		} else {
			panic(fmt.Sprintf("NOT SUPPORTED: don't know how to handle empty or array outputsource"))
		}
	}
	return nil
}

func (task *Task) setupOutputMap() error {
	task.outputIDMap = make(map[string]string)
	for _, step := range task.Root.Steps {
		for _, output := range step.Out {
			task.outputIDMap[output.ID] = step.ID
		}
	}
	return nil
}

func (task *Task) gatherScatterOutputs() (err error) {
	fmt.Println("gathering scatter outputs..")
	task.Outputs = make(cwl.Parameters)
	totalOutput := make([]cwl.Parameters, len(task.ScatterTasks))
	var wg sync.WaitGroup
	for _, scatterTask := range task.ScatterTasks {
		wg.Add(1)
		fmt.Printf("running goroutine for %v\n", scatterTask.ScatterIndex)
		go func(scatterTask *Task, totalOutput []cwl.Parameters) {
			fmt.Printf("in goroutine %v\n", scatterTask.ScatterIndex)
			defer wg.Done()
			fmt.Printf("entering while loop in goroutine %v\n", scatterTask.ScatterIndex)
			for len(scatterTask.Outputs) == 0 {
				// wait for scattered task to finish
				fmt.Printf("waiting for scattered task %v to finish..\n", scatterTask.ScatterIndex)
			}
			fmt.Printf("exited while loop in routine %v\n", scatterTask.ScatterIndex)
			totalOutput[scatterTask.ScatterIndex-1] = scatterTask.Outputs // note ScatterIndex begins at 1, not 0
		}(scatterTask, totalOutput)
	}
	wg.Wait()
	fmt.Println("assigning totalOutput from scattered process")
	task.Outputs[task.Root.Outputs[0].ID] = totalOutput // not sure what output ID to use here?
	return nil
}

// only one input means no scatterMethod
// if more than one input, must have scatterMethod `dotproduct` or `flat_crossproduct`
// nested_crossproduct scatterMethod not supported
func (task *Task) validateScatterMethod() (err error) {
	if len(task.Scatter) == 0 {
		// this check *might* be redundant - but just in case, keeping it for now
		return fmt.Errorf("no inputs to scatter")
	}
	if len(task.Scatter) == 1 && task.ScatterMethod != "" {
		return fmt.Errorf("scatterMethod specified but only one input to scatter")
	}
	if len(task.Scatter) > 1 && task.ScatterMethod == "" {
		return fmt.Errorf("more than one input to scatter but no scatterMethod specified")
	}
	if task.ScatterMethod == "nested_crossproduct" {
		return fmt.Errorf("scatterMethod \"nested_crossproduct\" not supported")
	}
	if len(task.Scatter) > 1 && task.ScatterMethod != "dotproduct" && task.ScatterMethod != "flat_crossproduct" {
		return fmt.Errorf("invalid scatterMethod: %v", task.ScatterMethod)
	}
	return nil
}

// returns boolean indicating whether all input params have same length
// and the length if true
func uniformLength(scatterParams map[string][]interface{}) (uniform bool, length int) {
	initLen := -1
	for _, v := range scatterParams {
		if initLen == -1 {
			initLen = len(v)
		}
		if len(v) != initLen {
			return false, 0
		}
	}
	return true, initLen
}

// assign input value to each scattered input parameter
func (task *Task) getScatterParams() (scatterParams map[string][]interface{}, err error) {
	scatterParams = make(map[string][]interface{})
	if err != nil {
		return nil, err
	}
	for _, scatterKey := range task.Scatter {
		input := task.Parameters[scatterKey]
		paramArray, ok := buildArray(input) // returns object of type []interface{}
		if !ok {
			return nil, fmt.Errorf("scatter on non-array input %v", scatterKey)
		}
		scatterParams[scatterKey] = paramArray
	}
	if task.ScatterMethod == "dotproduct" {
		// dotproduct requires that all scattered inputs have same length
		// uniformLength() returns true if all inputs have same length; false otherwise
		if ok, _ := uniformLength(scatterParams); !ok {
			return nil, fmt.Errorf("scatterMethod is dotproduct but not all inputs have same length")
		}
	}
	return scatterParams, nil
}

// assigns values to all non-scattered parameters
// the receiver task here is a subtask of a scattered task called `parentTask`
// see simpleScatter(), dotproduct(), flatCrossproduct()
func (task *Task) fillNonScatteredParams(parentTask *Task) {
	for param, val := range parentTask.Parameters {
		if _, ok := task.Parameters[param]; !ok {
			task.Parameters[param] = val
		}
	}
}

// should work but need to test
func (task *Task) dotproduct(scatterParams map[string][]interface{}) (err error) {
	// no need to check input lengths - this already got validated in Task.getScatterParams()
	_, inputLength := uniformLength(scatterParams)
	for i := 0; i < inputLength; i++ {
		subtask := &Task{
			JobID:        task.JobID,
			Engine:       task.Engine,
			Root:         task.Root,
			Parameters:   make(cwl.Parameters),
			originalStep: task.originalStep,
			ScatterIndex: i + 1, // count starts from 1, not 0, so that we can check if the ScatterIndex is nil (0 if nil)
		}
		// assign the i'th element of each input array as input to this scatter subtask
		for param, inputArray := range scatterParams {
			subtask.Parameters[param] = inputArray[i]
		}
		// assign values to all non-scattered parameters
		subtask.fillNonScatteredParams(task)
		task.ScatterTasks[i] = subtask
		fmt.Println("subtask parameters:")
		PrintJSON(subtask.Parameters)
	}
	return nil
}

func (task *Task) flatCrossproduct(scatterParams map[string][]interface{}) (err error) {
	return nil
}

// populates task.ScatterTasks with the scattered subtasks to be executed
// according to scatterMethod specified
func (task *Task) buildScatterTasks(scatterParams map[string][]interface{}) (err error) {
	fmt.Printf("\tBuilding scatter subtasks for %v input(s) with scatterMethod %v\n", len(scatterParams), task.ScatterMethod)
	task.ScatterTasks = make(map[int]*Task)
	switch task.ScatterMethod {
	// simple scattering over one input is a special case of dotproduct
	case "", "dotproduct":
		err = task.dotproduct(scatterParams)
		if err != nil {
			return err
		}
	case "flat_crossproduct":
		// scattering >=2 inputs
		err = task.flatCrossproduct(scatterParams)
		if err != nil {
			return err
		}
	}
	return nil
}

// run all scatter tasks concurrently
// maybe can apply this same logic to workflow engine
// so that all independent steps get run concurrently
func (task *Task) runScatterTasks() (err error) {
	fmt.Println("running scatter tasks concurrently..")
	var wg sync.WaitGroup
	for _, scattertask := range task.ScatterTasks {
		wg.Add(1)
		go func(scattertask *Task) {
			defer wg.Done()
			scattertask.Run()
		}(scattertask)
	}
	wg.Wait()
	return nil
}

// HERE TODO: implement scatter
func (task *Task) runScatter() (err error) {
	if err = task.validateScatterMethod(); err != nil {
		return err
	}
	scatterParams, err := task.getScatterParams()
	if err != nil {
		return err
	}
	err = task.buildScatterTasks(scatterParams)
	if err != nil {
		return err
	}
	err = task.runScatterTasks()
	if err != nil {
		return err
	}
	return nil
}

// for handling any kind of array/slice input to scatter
// need to convert whatever input we encounter to a generalized array of type []interface{}
// not sure if there is an easier way to do this
// see: https://stackoverflow.com/questions/14025833/range-over-interface-which-stores-a-slice
// ---
// if i is an array or slice  -> returns arr, true
// if i is not an array or slice -> return nil, false
func buildArray(i interface{}) (arr []interface{}, isArr bool) {
	kind := reflect.TypeOf(i).Kind()
	if kind != reflect.Array && kind != reflect.Slice {
		return nil, false
	}
	s := reflect.ValueOf(i)            // get underlying array
	arr = make([]interface{}, s.Len()) // allocate generalized array of same length
	for n := 0; n < s.Len(); n++ {
		arr[n] = s.Index(n).Interface() // retrieve each value by index from the input array
	}
	return arr, true
}

// Run a task
// a task is a process is a node on the graph
// a task can represent any of [Workflow, CommandLineTool, ExpressionTool, ...]
func (task *Task) Run() error {
	workflow := task.Root // use "process" instead of "workflow" as the variable name here
	params := task.Parameters

	fmt.Printf("\nRunning task: %v\n", workflow.ID)
	if task.Scatter != nil {
		task.runScatter()
		fmt.Println("between running scatter and gathering scatter outputs")
		task.gatherScatterOutputs()
		return nil // stop processing scatter task
	}

	// if this process is a workflow
	// it is recursively resolved to a collection of *Tools
	// *Tools require no processing - they get dispatched to the task engine
	// *Tools are the leaves in the graph - the actual commands to be executed for the top-level workflow job
	if workflow.Class == "Workflow" {
		// create an unfinished steps map as a queue
		// a collection of the stepIDs for the steps of this workflow
		// stored in task.unFinishedSteps
		task.setupStepQueue()

		// store a map of {outputID: stepID} pairs to trace dependency
		task.setupOutputMap()

		var curStepID string
		var prevStepID string
		var curStep cwl.Step
		//  pick random step
		curStepID = task.getStep()

		// while there are unfinished steps
		for len(task.unFinishedSteps) > 0 {
			fmt.Printf("\tProcessing Step: %v\n", curStepID)
			prevStepID = ""

			subtask, ok := task.Children[curStepID] // retrieve task object for this step (subprocess) of the workflow
			if !ok {
				panic(fmt.Sprintf("can't find workflow %v", curStepID))
			}
			curStep = subtask.originalStep // info about this subprocess from the parent process' step list

			/*
				curStep.In example:

					"in": [
							{
									"source": "#subworkflow_test.cwl/test_initworkdir/bam_with_index",
									"valueFrom": "$([self, self.secondaryFiles[0]])",
									"id": "#subworkflow_test.cwl/test_expr/file_array"
							}
					]
			*/

			idMaps := make(map[string]string)
			for _, input := range curStep.In {
				subtaskInput := step2taskID(&curStep, input.ID)
				idMaps[input.ID] = subtaskInput // step input ID maps to [sub]task input ID
				for _, source := range input.Source {
					// P: if source is an ID that points to an output in another step
					if stepID, ok := task.outputIDMap[source]; ok {
						if _, ok := task.unFinishedSteps[stepID]; ok {
							prevStepID = stepID
							break
						} else {
							// assign output parameter of dependency step (which has already been executed) to input parameter of this step
							// HERE need to check engine stack to see if the dependency step has completed
							// how will this logic work - there's kind of a delay here, waiting for the dependency task to run
							// maybe a while-loop which loops until depTask has its output populated
							// but I don't want to block the rest of processing task.Run()
							// maybe there could be a go routine somewhere in here so non-dependent steps can run without waiting for each other
							depTask := task.Children[stepID]
							outputID := depTask.Root.ID + strings.TrimPrefix(source, stepID)
							inputPresent := false
							for ; !inputPresent; _, inputPresent = subtask.Parameters[subtaskInput] {
								fmt.Println("\tWaiting for dependency task to finish running..")
								if len(depTask.Outputs) > 0 {
									fmt.Println("\tDependency task complete!")
									subtask.Parameters[subtaskInput] = depTask.Outputs[outputID]
									fmt.Println("\tSuccessfully collected output from dependency task.")
								}
								time.Sleep(2 * time.Second) // for testing..
							}
						}
					} else if strings.HasPrefix(source, workflow.ID) {
						// if the input source to this step is not the outputID of another step
						// but is an input of the parent workflow (e.g. "#subworkflow_test.cwl/input_bam" in gen3_test.pack.cwl)
						// assign input parameter of parent workflow ot input parameter of this step

						// P: step.in.id is composed of {stepID}/{inputID}
						// P: it's mapped to the step's workflow's input definition
						// P: which has the structure of {stepWorkflowID}/{inputID}
						subtask.Parameters[subtaskInput] = params[source]
					}

				}
				if prevStepID != "" {
					// if we found a step dependency, then stop handling for this current step
					break
				}
			}

			// P: cancel processing this step, go to next loop to process dependent step
			if prevStepID != "" {
				curStepID = prevStepID
				fmt.Printf("\tUnresolved dependency! Going to dependency step: %v\n", curStepID)
				continue
			}

			// reaching here implies one of <no step dependency> or <all step dependencies have been resolved/handled/run>

			if len(curStep.Scatter) > 0 {
				// subtask.Scatter = make([]string, len(curStep.Scatter))
				for _, i := range curStep.Scatter {
					subtask.Scatter = append(subtask.Scatter, idMaps[i])
				}
			}
			subtask.Run()

			delete(task.unFinishedSteps, curStepID)
			// get random next step
			curStepID = task.getStep()
		}
		fmt.Println("\t\tMerging outputs for task ", task.Root.ID)
		task.mergeChildOutputs() // for workflows only - merge outputs from all steps of this workflow to output for this workflow
	} else {
		// this process is not a workflow - it is a leaf in the graph (a *Tool) and gets dispatched to the task engine
		if task.Scatter != nil {
			fmt.Printf("\tI am going to scatter this!!\n")
		}
		fmt.Printf("Dispatching task %v..\n", task.Root.ID)
		task.Engine.DispatchTask(task.JobID, task)
	}
	return nil
}
