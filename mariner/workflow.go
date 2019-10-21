package mariner

import (
	"encoding/json"
	"fmt"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains functions for managing the workflow graph
// i.e., assemble the graph, track dependencies
// recursively process workflows into Tools
// dispatch Tools to be executed by the K8sEngine

// NOTE: workflow steps are processed concurrently - see RunSteps()

// load in config from `mariner-config.json`
// which is a configmap object in the k8s cluster with name `mariner-config`
// NOTE: when moving stuff to cloud automation,
// ----- probably the config will be put in the manifest which holds the config for all the other services
// ----- and the configmap name might change to `manifest-mariner`
// ----- when this happens, need to update 1. mariner-config.json 2. mariner-deploy.yaml 3. engine job spec (DispatchWorkflowJob)
var Config = loadConfig("/mariner-config/mariner-config.json")

/*
 	a Task is a process is a node on the graph is one of [Workflow, CommandLineTool, ExpressionTool, ...]

	Task is a nested object
	A Task represents a process, which is a node on the graph (NOTE: maybe rename Task to Process, or something)
	So a Task is either a leaf in the graph (a Tool)
	or not a leaf in the graph (a workflow)
	If a Task is a workflow, then it has steps
	These steps have their own representations as Task objects
	The Task objects of the steps of a workflow are stored in the Children field of the workflow's Task object

	So for a workflow Task:
	task.Children is a map, where keys are the taskIDs and values are the Task objects of the workflow steps
*/
type Task struct {
	Parameters    cwl.Parameters    // input parameters of this task
	Root          *cwl.Root         // "root" of the "namespace" of the cwl file for this task
	Outputs       cwl.Parameters    // output parameters of this task
	Scatter       []string          // if task is a step in a workflow and requires scatter; input parameters to scatter are stored here
	ScatterMethod string            // if task is step in a workflow and requires scatter; scatter method specified - "dotproduct" or "flatcrossproduct" or ""
	ScatterTasks  map[int]*Task     // if task is a step in a workflow and requires scatter; scattered subtask objects stored here; scattered subtasks are enumerated
	ScatterIndex  int               // if a task gets scattered, each subtask belonging to that task gets enumerated, and that index is stored here
	Children      map[string]*Task  // if task is a workflow; the Task objects of the workflow steps are stored here; {taskID: task} pairs
	OutputIDMap   map[string]string // if task is a workflow; a map of {outputID: stepID} pairs in order to trace i/o dependencies between steps
	OriginalStep  cwl.Step          // if this task is a step in a workflow, this is the information from this task's step entry in the parent workflow's cwl file
	Done          *bool             // false until all output for this task has been collected, then true
	// --- New Fields ---
	Log *Log // contains Status, Stats, Event
}

// recursively populates `mainTask` (the task object for the top level workflow with all downstream task objects)
// see note describing the Task type for explanation of nested structure of Task
// basically, if task is a workflow, the task objects for the workflow steps get stored in the Task.Children field
// so the graph gets "resolved" via creating one big task (`mainTask`) which contains the entire workflow
// i.e., the whole workflow and its graphical structure are represented as a nested collection of Task objects
func (engine *K8sEngine) resolveGraph(rootMap map[string]*cwl.Root, curTask *Task) error {
	if curTask.Root.Class == "Workflow" {
		curTask.Children = make(map[string]*Task)
		for _, step := range curTask.Root.Steps {
			subworkflow, ok := rootMap[step.Run.Value]
			if !ok {
				panic(fmt.Sprintf("can't find workflow %v", step.Run.Value))
			}
			newTask := &Task{
				Root:         subworkflow,
				Parameters:   make(cwl.Parameters),
				Outputs:      make(cwl.Parameters),
				OriginalStep: step,
				Done:         &falseVal,
				Log:          logger(), // pointer to Log struct with status NOT_STARTED
			}
			// FIXME - this can probably be cleaned up
			engine.Log.ByProcess[step.ID] = newTask.Log

			// TODO - MONDAY - capture desired I/O in log
			/*
				- inputs
				- outputs
				- stats
					- time
					- resources
					- failures
					- retries
			*/

			// FIXME - I want a map of input parameter to VALUE - provided
			// need to write a few lines to do this, in loadInputs (?)
			newTask.Log.Input = make(map[string]*cwl.Provided)

			// FIXME - empty??
			newTask.Log.Output = newTask.Outputs

			engine.resolveGraph(rootMap, newTask)
			// what to use as id? value or step.id - using step.ID for now, seems to work okay
			curTask.Children[step.ID] = newTask
		}
	}
	return nil
}

// RunWorkflow parses a workflow and inputs and run it
func (engine *K8sEngine) runWorkflow(workflow []byte, inputs []byte) error {
	var root cwl.Root
	err := json.Unmarshal(workflow, &root) // unmarshal the packed workflow JSON from the request body
	if err != nil {
		return err
	}
	var originalParams cwl.Parameters
	err = json.Unmarshal(inputs, &originalParams) // unmarshal the inputs JSON from the request body
	if err != nil {
		return err
	}

	// small preprocessing step to get the right input param IDs for the top level workflow
	var params = make(cwl.Parameters)
	for id, value := range originalParams {
		params["#main/"+id] = value
	}

	// Task object for top level workflow, later to be recursively populated
	// with task objects for all the other nodes in the workflow graph
	var mainTask *Task

	// a flat map to store all the "root" objects (basically a representation of a CWL file) which comprise the workflow
	// e.g., if I have a CWL workflow comprised of three files:
	// 1. workflow.cwl 2. stepA.cwl 3. stepB.cwl
	// there would be three "root" objects - one for each CWL file
	// and all the information in a CWL file is serialized/stored in the "root" object for that file
	flatRoots := make(map[string]*cwl.Root)

	// iterate through master list of all process objects in packed cwl json, populate flatRoots
	for _, process := range root.Graphs {
		flatRoots[process.ID] = process
		// once we encounter the top level workflow (which always has ID "#main")
		if process.ID == "#main" {
			// construct `mainTask` - the task object for the top level workflow
			mainTask = &Task{
				Root:       process,
				Parameters: params,
				Log:        logger(), // initialize empty Log object with status NOT_STARTED
			}
		}
	}
	if mainTask == nil {
		panic(fmt.Sprint("can't find main workflow"))
	}

	// recursively populate `mainTask` with Task objects for the rest of the nodes in the workflow graph
	engine.resolveGraph(flatRoots, mainTask)

	// run the workflow
	engine.run(mainTask)

	// FIXME - probably don't put this here
	engine.Log.Engine.Status = COMPLETE
	engine.Log.write()

	// if this works I'm gonna be stoked in all aspects
	fmt.Println("Here's the log file!")
	showLog(engine.Log.Path)

	fmt.Print("\n\nFinished running workflow job.\n")
	fmt.Println("Here's the output:")
	printJSON(mainTask.Outputs)

	return nil
}

/*
concurrency notes:
1. Each step needs to wait until its input params are all populated before .Run()
2. mergeChildOutputs() needs to wait until the outputs are actually there to collect them - wait until the steps have finished running
*/

// Run recursively and concurrently processes Tasks
// recall: a Task is either a workflow or a Tool
// workflows are processed into a collection of Tools via Task.RunSteps()
// Tools get dispatched to be executed via Task.Engine.DispatchTask()
func (engine *K8sEngine) run(task *Task) error {
	fmt.Printf("\nRunning task: %v\n", task.Root.ID)
	task.Log.Status = IN_PROGRESS
	engine.Log.write()
	if task.Scatter != nil {
		engine.runScatter(task)
		engine.gatherScatterOutputs(task)
		return nil
	}
	if task.Root.Class == "Workflow" {
		fmt.Printf("Handling workflow %v..\n", task.Root.ID)
		engine.runSteps(task)
		task.mergeChildOutputs()
	} else {
		// this process is not a workflow - it is a leaf in the graph (a Tool) and gets dispatched to the task engine
		fmt.Printf("Dispatching task %v..\n", task.Root.ID)
		engine.dispatchTask(task)
	}
	task.Log.Status = COMPLETE
	engine.Log.write()
	return nil
}

// for concurrent processing of steps of a workflow
// key point: the task does not get Run() until its input params are populated - that's how/where the dependencies get handled
func (engine *K8sEngine) runStep(curStepID string, parentTask *Task, task *Task) {
	fmt.Printf("\tProcessing Step: %v\n", curStepID)
	curStep := task.OriginalStep
	idMaps := make(map[string]string)
	for _, input := range curStep.In {
		taskInput := step2taskID(&curStep, input.ID)
		idMaps[input.ID] = taskInput // step input ID maps to [sub]task input ID

		// presently not handling the case of multiple sources for a given input parameter
		// see: https://www.commonwl.org/v1.0/Workflow.html#WorkflowStepInput
		// the section on "Merging", with the "MultipleInputFeatureRequirement" and "linkMerge" fields specifying either "merge_nested" or "merge_flattened"
		source := input.Source[0]

		// I/O DEPENDENCY HANDLING
		// if this input's source is the ID of an output parameter of another step
		if depStepID, ok := parentTask.OutputIDMap[source]; ok {
			// wait until all dependency step output has been collected
			// and then assign output parameter of dependency step (which has just finished running) to input parameter of this step
			depTask := parentTask.Children[depStepID]
			outputID := depTask.Root.ID + strings.TrimPrefix(source, depStepID)

			fmt.Println("\tWaiting for dependency task to finish running..")
			for inputPresent := false; !inputPresent; _, inputPresent = task.Parameters[taskInput] {
				if *depTask.Done {
					fmt.Println("\tDependency task complete!")
					task.Parameters[taskInput] = depTask.Outputs[outputID]
					fmt.Println("\tSuccessfully collected output from dependency task.")
				}
			}
		} else if strings.HasPrefix(source, parentTask.Root.ID) {
			// if the input source to this step is not the outputID of another step
			// but is an input of the parent workflow
			// assign input parameter of parent workflow to input parameter of this step
			task.Parameters[taskInput] = parentTask.Parameters[source]
		}
	}

	// reaching here implies one of
	// 1. there are no step dependencies, or
	// 2. all step dependencies have been resolved/handled/run

	// if this step specifies input parameters to be scattered
	// collect those parameters in Task.Scatter array
	if len(curStep.Scatter) > 0 {
		for _, i := range curStep.Scatter {
			task.Scatter = append(task.Scatter, idMaps[i])
		}
	}

	// run this step
	engine.run(task)
}

// concurrently run steps of a workflow
func (engine *K8sEngine) runSteps(task *Task) {
	// store a map of {outputID: stepID} pairs to trace step i/o dependency
	task.setupOutputMap()
	// NOTE: not sure if this should have a WaitGroup - seems to work fine without one
	for curStepID, subtask := range task.Children {
		go engine.runStep(curStepID, task, subtask)
	}
}

// "#expressiontool_test.cwl" + "[#subworkflow_test.cwl]/test_expr/file_array"
// returns "#expressiontool_test.cwl/test_expr/file_array"
func step2taskID(step *cwl.Step, stepVarID string) string {
	return step.Run.Value + strings.TrimPrefix(stepVarID, step.ID)
}

// only called if task is a workflow
// mergeChildOutputs maps outputs from the workflow's step tasks to the workflow task's output parameters
// i.e., task.Outputs is a map of (outputID, outputValue) pairs for all the outputs of this workflow
// where outputID is an output of the workflow AND an output of one of the steps of the workflow
// and outputValue is the value for that output parameter for the workflow step
// -> this outputValue gets mapped from the workflow step's outputs to the output of the workflow itself
func (task *Task) mergeChildOutputs() error {
	task.Outputs = make(cwl.Parameters)
	if task.Children == nil {
		panic(fmt.Sprintf("Can't call merge child outputs without childs %v \n", task.Root.ID))
	}
	for _, output := range task.Root.Outputs {
		if len(output.Source) == 1 {
			source := output.Source[0]
			stepID, ok := task.OutputIDMap[source]
			if !ok {
				panic(fmt.Sprintf("Can't find output source %v", source))
			}
			subtaskOutputID := step2taskID(&task.Children[stepID].OriginalStep, source)
			fmt.Printf("Waiting to merge child outputs for workflow %v ..\n", task.Root.ID)
			for outputPresent := false; !outputPresent; _, outputPresent = task.Outputs[output.ID] {
				if outputVal, ok := task.Children[stepID].Outputs[subtaskOutputID]; ok {
					task.Outputs[output.ID] = outputVal
				}
			}
		} else {
			panic(fmt.Sprintf("NOT SUPPORTED: don't know how to handle empty or array outputsource"))
		}
	}
	return nil
}

// for when task is a workflow
// map of {output.ID: step.ID} pairs
// in order to trace dependencies among steps in a workflow
// e.g., if the outputID of one step is the "source" of the input of another step
// -> that's a dependency between steps,
// and so the dependency step must finish running
// before the dependent step can execute
func (task *Task) setupOutputMap() error {
	task.OutputIDMap = make(map[string]string)
	for _, step := range task.Root.Steps {
		for _, output := range step.Out {
			task.OutputIDMap[output.ID] = step.ID
		}
	}
	return nil
}
