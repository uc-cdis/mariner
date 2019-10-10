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

func (task *Task) gatherScatterOutputs() (err error) {
	fmt.Println("gathering scatter outputs..")
	task.Outputs = make(cwl.Parameters)
	totalOutput := make([]cwl.Parameters, len(task.ScatterTasks))
	var wg sync.WaitGroup
	for _, scatterTask := range task.ScatterTasks {
		wg.Add(1)
		go func(scatterTask *Task, totalOutput []cwl.Parameters) {
			defer wg.Done()
			for !*scatterTask.Done {
				// wait for scattered task to finish
				// fmt.Printf("waiting for scattered task %v to finish..\n", scatterTask.ScatterIndex)
			}
			totalOutput[scatterTask.ScatterIndex-1] = scatterTask.Outputs // note: ScatterIndex begins at 1, not 0
		}(scatterTask, totalOutput)
	}
	wg.Wait()
	task.Outputs[task.Root.Outputs[0].ID] = totalOutput // not sure what output ID to use here (?)
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

// run all scatter tasks concurrently
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
	// fmt.Printf("\tBuilding scatter subtasks for %v input(s) with scatterMethod %v\n", len(scatterParams), task.ScatterMethod)
	task.ScatterTasks = make(map[int]*Task)
	switch task.ScatterMethod {
	case "", "dotproduct": // simple scattering over one input is a special case of dotproduct
		err = task.dotproduct(scatterParams)
		if err != nil {
			return err
		}
	case "flat_crossproduct":
		err = task.flatCrossproduct(scatterParams)
		if err != nil {
			return err
		}
	}
	return nil
}

// see dotproduct and flatCrossproduct descriptions in this section of cwl docs: https://www.commonwl.org/v1.0/Workflow.html#WorkflowStep
func (task *Task) dotproduct(scatterParams map[string][]interface{}) (err error) {
	// no need to check input lengths - this already got validated in Task.getScatterParams()
	_, inputLength := uniformLength(scatterParams)
	for i := 0; i < inputLength; i++ {
		subtask := &Task{
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
	}
	return nil
}

// get cartesian product of input arrays
// tested algorithm in goplayground: https://play.golang.org/p/jiN5uP08rnm
func (task *Task) flatCrossproduct(scatterParams map[string][]interface{}) (err error) {
	paramIDList := make([]string, 0, len(scatterParams))
	inputArrays := make([][]interface{}, 0, len(scatterParams))
	for paramID, inputArray := range scatterParams {
		paramIDList = append(paramIDList, paramID)
		inputArrays = append(inputArrays, inputArray)
	}

	lens := func(i int) int { return len(inputArrays[i]) }

	scatterIndex := 1
	for ix := make([]int, len(inputArrays)); ix[0] < lens(0); nextIndex(ix, lens) {
		subtask := &Task{
			Engine:       task.Engine,
			Root:         task.Root,
			Parameters:   make(cwl.Parameters),
			originalStep: task.originalStep,
			ScatterIndex: scatterIndex, // count starts from 1, not 0, so that we can check if the ScatterIndex is nil (0 if nil)
		}
		for j, k := range ix {
			subtask.Parameters[paramIDList[j]] = inputArrays[j][k]
		}
		subtask.fillNonScatteredParams(task)
		task.ScatterTasks[scatterIndex] = subtask
		scatterIndex++
	}
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
	for param, val := range parentTask.Parameters {
		if _, ok := task.Parameters[param]; !ok {
			task.Parameters[param] = val
		}
	}
}
