package mariner

import (
	"fmt"
	"reflect"
	"sync"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for processing scattered workflow steps
// NOTE: scattered subtasks get run concurrently -> see runScatterTasks()
// what does "scatter" mean? great question -> see: https://www.commonwl.org/v1.0/Workflow.html#WorkflowStep
func (engine *K8sEngine) runScatter(task *Task) (err error) {
	engine.infof("begin run scatter for task: %v", task.Root.ID)
	if err = task.validateScatterMethod(); err != nil {
		// probably this really means "invalid", not "error"
		// fixme
		return engine.errorf("failed at scatter method validation for task: %v; error: %v", task.Root.ID, err)
	}
	scatterParams, err := task.scatterParams()
	if err != nil {
		return engine.errorf("failed to load scatter params for task: %v; error: %v", task.Root.ID, err)
	}
	err = task.buildScatterTasks(scatterParams)
	if err != nil {
		return engine.errorf("failed to build subtasks for scatter task: %v; error: %v", task.Root.ID, err)
	}
	err = engine.runScatterTasks(task)
	if err != nil {
		return engine.errorf("failed to run subtasks for scatter task: %v; error: %v", task.Root.ID, err)
	}
	engine.infof("end run scatter for task: %v", task.Root.ID)
	return nil
}

// assign input value to each scattered input parameter
func (task *Task) scatterParams() (scatterParams map[string][]interface{}, err error) {
	task.infof("begin load scatter params")
	scatterParams = make(map[string][]interface{})
	for _, scatterKey := range task.Scatter {
		task.infof("begin handle scatter param: %v", scatterKey)

		// debug gwas
		fmt.Printf("begin handle scatter param: %v", scatterKey)
		fmt.Println("task.Parameters:")
		printJSON(task.Parameters)
		fmt.Println("task.Scatter:")
		printJSON(task.Scatter)

		input := task.Parameters[scatterKey]
		paramArray, ok := buildArray(input) // returns object of type []interface{}
		if !ok {
			return nil, task.errorf("scatter on non-array input %v", scatterKey)
		}
		scatterParams[scatterKey] = paramArray
		task.infof("end handle scatter param: %v", scatterKey)
	}
	if task.ScatterMethod == "dotproduct" {
		// dotproduct requires that all scattered inputs have same length
		// uniformLength() returns true if all inputs have same length; false otherwise
		if ok, _ := uniformLength(scatterParams); !ok {
			return nil, task.errorf("scatterMethod is dotproduct but not all inputs have same length")
		}
	}
	task.infof("end load scatter params")
	return scatterParams, nil
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

func (engine *K8sEngine) gatherScatterOutputs(task *Task) (err error) {
	engine.infof("begin gather scatter outputs for task: %v", task.Root.ID)
	task.Outputs = make(map[string]interface{})
	totalOutput := make(map[string][]interface{})
	for _, param := range task.Root.Outputs {
		totalOutput[param.ID] = make([]interface{}, len(task.ScatterTasks))
	}
	var wg sync.WaitGroup
	for _, scatterTask := range task.ScatterTasks {
		wg.Add(1)
		// HERE - add sync.Lock mechanism for safe concurrent writing to map
		go func(scatterTask *Task, totalOutput map[string][]interface{}) {
			defer wg.Done()
			for !*scatterTask.Done {
				// wait for scattered task to finish
				// fmt.Printf("waiting for scattered task %v to finish..\n", scatterTask.ScatterIndex)
			}
			for _, param := range task.Root.Outputs {
				totalOutput[param.ID][scatterTask.ScatterIndex-1] = scatterTask.Outputs[param.ID]
			}
		}(scatterTask, totalOutput)
	}
	wg.Wait()
	for param, val := range totalOutput {
		task.Outputs[param] = val
	}
	task.Log.Output = task.Outputs
	engine.infof("end gather scatter outputs for task: %v", task.Root.ID)
	return nil
}

// only one input means no scatterMethod
// if more than one input, must have scatterMethod `dotproduct` or `flat_crossproduct`
// nested_crossproduct scatterMethod not supported
func (task *Task) validateScatterMethod() (err error) {
	task.infof("begin validate scatter method")

	// populate task ScatterMethod field here
	task.ScatterMethod = task.OriginalStep.ScatterMethod

	// gwas debug
	fmt.Println("begin validate scatter method for task:")
	printJSON(task)

	if len(task.Scatter) == 0 {
		// this check *might* be redundant - but just in case, keeping it for now
		// fixme - double check - this might not be an error actually
		return task.errorf("no inputs to scatter")
	}
	if len(task.Scatter) == 1 && task.ScatterMethod != "" {
		return task.errorf("scatterMethod specified but only one input to scatter")
	}
	if len(task.Scatter) > 1 && task.ScatterMethod == "" {
		return task.errorf("more than one input to scatter but no scatterMethod specified")
	}
	if task.ScatterMethod == "nested_crossproduct" {
		return task.errorf("scatterMethod \"nested_crossproduct\" not supported")
	}
	if len(task.Scatter) > 1 && task.ScatterMethod != "dotproduct" && task.ScatterMethod != "flat_crossproduct" {
		return task.errorf("invalid scatterMethod: %v", task.ScatterMethod)
	}
	task.infof("end validate scatter method")
	return nil
}

// run all scatter tasks concurrently
func (engine *K8sEngine) runScatterTasks(task *Task) (err error) {
	engine.infof("begin run subtasks for scatter task: %v", task.Root.ID)
	var wg sync.WaitGroup
	for _, scattertask := range task.ScatterTasks {
		wg.Add(1)
		go func(scattertask *Task) {
			defer wg.Done()
			engine.run(scattertask)
		}(scattertask)
	}
	wg.Wait()
	engine.infof("end run subtasks for scatter task: %v", task.Root.ID)
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

// populates task.ScatterTasks with scattered subtasks according to scatterMethod
func (task *Task) buildScatterTasks(scatterParams map[string][]interface{}) (err error) {
	task.infof("begin build scatter subtasks for %v input(s) with scatterMethod %v", len(scatterParams), task.ScatterMethod)
	task.ScatterTasks = make(map[int]*Task)
	task.Log.Scatter = make(map[int]*Log)
	switch task.ScatterMethod {
	case "", "dotproduct": // simple scattering over one input is a special case of dotproduct
		err = task.dotproduct(scatterParams)
		if err != nil {
			return task.errorf("%v", err)
		}
	case "flat_crossproduct":
		err = task.flatCrossproduct(scatterParams)
		if err != nil {
			return task.errorf("%v", err)
		}
	}
	task.infof("end build scatter subtasks")
	return nil
}

// see dotproduct and flatCrossproduct descriptions in this section of cwl docs: https://www.commonwl.org/v1.0/Workflow.html#WorkflowStep
func (task *Task) dotproduct(scatterParams map[string][]interface{}) (err error) {
	task.infof("begin build scatter subtasks by dotproduct method")
	// no need to check input lengths - this already got validated in Task.getScatterParams()
	_, inputLength := uniformLength(scatterParams)
	for i := 0; i < inputLength; i++ {
		task.infof("begin build subtask %v", i)
		subtask := &Task{
			Root:         task.Root,
			Parameters:   make(cwl.Parameters),
			OriginalStep: task.OriginalStep,
			Done:         &falseVal,
			Log:          logger(),
			ScatterIndex: i + 1, // count starts from 1, not 0, so that we can check if the ScatterIndex is nil (0 if nil)
		}
		// assign the i'th element of each input array as input to this scatter subtask
		for param, inputArray := range scatterParams {
			task.infof("assigning val %v to param %v", inputArray[i], param)
			subtask.Parameters[param] = inputArray[i]
		}
		// assign values to all non-scattered parameters
		subtask.fillNonScatteredParams(task)
		task.ScatterTasks[i] = subtask

		// currently logging scattered tasks this way
		// the subtask logs are beneath/within the scatter task log object
		task.Log.Scatter[i] = subtask.Log
		task.infof("end build subtask %v", i)
	}
	task.infof("end build scatter subtasks by dotproduct method")
	return nil
}

// get cartesian product of input arrays
// tested algorithm in goplayground: https://play.golang.org/p/jiN5uP08rnm
func (task *Task) flatCrossproduct(scatterParams map[string][]interface{}) (err error) {
	task.infof("begin build scatter subtasks by flat_crossproduct method")
	paramIDList := make([]string, 0, len(scatterParams))
	inputArrays := make([][]interface{}, 0, len(scatterParams))
	for paramID, inputArray := range scatterParams {
		paramIDList = append(paramIDList, paramID)
		inputArrays = append(inputArrays, inputArray)
	}

	lens := func(i int) int { return len(inputArrays[i]) }

	scatterIndex := 1
	for ix := make([]int, len(inputArrays)); ix[0] < lens(0); nextIndex(ix, lens) {
		task.infof("begin build subtask %v", scatterIndex)
		subtask := &Task{
			Root:         task.Root,
			Parameters:   make(cwl.Parameters),
			OriginalStep: task.OriginalStep,
			Done:         &falseVal,
			Log:          logger(),
			ScatterIndex: scatterIndex, // count starts from 1, not 0, so that we can check if the ScatterIndex is nil (0 if nil)
		}
		for j, k := range ix {
			task.infof("assigning val %v to param %v", inputArrays[j][k], paramIDList[j])
			subtask.Parameters[paramIDList[j]] = inputArrays[j][k]
		}
		subtask.fillNonScatteredParams(task)
		task.ScatterTasks[scatterIndex] = subtask

		// currently logging scattered tasks this way
		// the subtask logs are beneath/within the scatter task log object
		task.Log.Scatter[scatterIndex] = subtask.Log

		task.infof("end build subtask %v", scatterIndex)
		scatterIndex++
	}
	task.infof("end build scatter subtasks by flat_crossproduct method")
	return nil
}

// used in flatCrossproduct()
// nextIndex sets ix to the lexicographically next value,
// such that for each i>0, 0 <= ix[i] < lens(i).
func nextIndex(ix []int, lens func(i int) int) {
	for j := len(ix) - 1; j >= 0; j-- {
		ix[j]++
		if j == 0 || ix[j] < lens(j) {
			return
		}
		ix[j] = 0
	}
}

// assigns values to all non-scattered parameters
// the receiver task here is a subtask of a scattered task called `parentTask`
// see simpleScatter(), dotproduct(), flatCrossproduct()
func (task *Task) fillNonScatteredParams(parentTask *Task) {
	task.infof("begin fill non-scattered params")
	for param, val := range parentTask.Parameters {
		if _, ok := task.Parameters[param]; !ok {
			task.infof("assigning val %v to non-scattered param %v", val, param)
			task.Parameters[param] = val
		}
	}
	task.infof("end fill non-scattered params")
}
