package mariner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains functions for managing the workflow graph
// i.e., assemble the graph, track dependencies
// recursively process workflows into *Tools
// dispatch *Tools to be executed by the K8sEngine

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
	So a Task is either a leaf in the graph (a *Tool)
	or not a leaf in the graph (a workflow)
	If a Task is a workflow, then it has steps
	These steps have their own representations as Task objects
	The Task objects of the steps of a workflow are stored in the Children field of the workflow's Task object

	So for a workflow Task:
	task.Children is a map, where keys are the taskIDs and values are the Task objects of the workflow steps
*/
type Task struct {
	Engine        *K8sEngine        // the workflow engine - all tasks in a workflow job share the same engine
	JobID         string            // pretty sure (?) this is the workflow k8s job ID - need to double check
	Parameters    cwl.Parameters    // input parameters of this task
	Root          *cwl.Root         // "root" of the "namespace" of the cwl file for this task
	Outputs       cwl.Parameters    // output parameters of this task
	Scatter       []string          // if task is a step in a workflow and requires scatter; input parameters to scatter are stored here
	ScatterMethod string            // if task is step in a workflow and requires scatter; scatter method specified - "dotproduct" or "flatcrossproduct" or ""
	ScatterTasks  map[int]*Task     // if task is a step in a workflow and requires scatter; scattered subtask objects stored here; scattered subtasks are enumerated
	ScatterIndex  int               // if a task gets scattered, each subtask belonging to that task gets enumerated, and that index is stored here
	Children      map[string]*Task  // if task is a workflow; the Task objects of the workflow steps are stored here; {taskID: task} pairs
	outputIDMap   map[string]string // if task is a workflow; a map of {outputID: stepID} pairs in order to trace i/o dependencies between steps
	originalStep  cwl.Step          // if this task is a step in a workflow, this is the information from this task's step entry in the parent workflow's cwl file
}

// recursively populates `mainTask` (the task object for the top level workflow with all downstream task objects)
// see note describing the Task type for explanation of nested structure of Task
// basically, if task is a workflow, the task objects for the workflow steps get stored in the Task.Children field
// so the graph gets "resolved" via creating one big task (`mainTask`) which contains the entire workflow
// i.e., the whole workflow and its graphical structure are represented as a nested collection of Task objects
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
			// what to use as id? value or step.id - using step.ID for now, seems to work okay
			curTask.Children[step.ID] = newTask
		}
	}
	return nil
}

// RunWorkflow parses a workflow and inputs and run it
func RunWorkflow(jobID string, workflow []byte, inputs []byte, engine *K8sEngine) error {
	var root cwl.Root
	err := json.Unmarshal(workflow, &root) // unmarshal the packed workflow JSON from the POST request body
	if err != nil {
		fmt.Println("failed to unmarshal workflow")
		return err
	}

	var originalParams cwl.Parameters
	err = json.Unmarshal(inputs, &originalParams) // unmarshal the inputs JSON from the POST request body
	if err != nil {
		fmt.Println("failed to unmarshal inputs")
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
			mainTask = &Task{JobID: jobID, Root: process, Parameters: params, Engine: engine}
		}
	}
	if mainTask == nil {
		panic(fmt.Sprint("can't find main workflow"))
	}

	// recursively populate `mainTask` with Task objects for the rest of the nodes in the workflow graph
	resolveGraph(flatRoots, mainTask)

	// run the workflow
	mainTask.Run()

	fmt.Print("\n\nFinished running workflow job.\n")
	fmt.Println("Here's the output:")
	PrintJSON(mainTask.Outputs)

	fmt.Println("Here is the current working directory of the engine:")
	fmt.Println(os.Getwd())
	fmt.Println("Switching dirs..")
	os.Chdir("/")
	fmt.Println("New working dir")
	fmt.Println(os.Getwd())
	return nil
}

/*
concurrency notes:
1. Each step needs to wait until its input params are all populated before .Run()
2. mergeChildOutputs() needs to wait until the outputs are actually there to collect them - wait until the steps have finished running
*/

// Run recursively and concurrently processes Tasks
// recall: a Task is either a workflow or a *Tool
// workflows are processed into a collection of *Tools via Task.RunSteps()
// *Tools get dispatched to be executed via Task.Engine.DispatchTask()
func (task *Task) Run() error {
	fmt.Printf("\nRunning task: %v\n", task.Root.ID)
	if task.Scatter != nil {
		task.runScatter()
		task.gatherScatterOutputs()
		return nil
	}

	if task.Root.Class == "Workflow" {
		// this process is a workflow, i.e., it has steps that must be run
		fmt.Printf("Handling workflow %v..\n", task.Root.ID)
		// concurrently run each of the workflow steps
		task.RunSteps()
		// merge outputs from all steps of this workflow to output for this workflow
		task.mergeChildOutputs()
	} else {
		// this process is not a workflow - it is a leaf in the graph (a *Tool) and gets dispatched to the task engine
		fmt.Printf("Dispatching task %v..\n", task.Root.ID)
		task.Engine.DispatchTask(task.JobID, task) // this line looks weird - task on left and right
	}
	return nil
}

// for concurrent processing of steps of a workflow
// key point: the task does not get Run() until its input params are populated - that's how/where the dependencies get handled
func (task *Task) runStep(curStepID string, parentTask *Task) {
	fmt.Printf("\tProcessing Step: %v\n", curStepID)
	curStep := task.originalStep
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
		if depStepID, ok := parentTask.outputIDMap[source]; ok {
			// wait until dependency step output is there
			// and then assign output parameter of dependency step (which has just finished running) to input parameter of this step
			depTask := parentTask.Children[depStepID]
			outputID := depTask.Root.ID + strings.TrimPrefix(source, depStepID)
			fmt.Println("\tWaiting for dependency task to finish running..")
			for inputPresent := false; !inputPresent; _, inputPresent = task.Parameters[taskInput] {
				if len(depTask.Outputs) > 0 {
					fmt.Println("\tDependency task complete!")
					task.Parameters[taskInput] = depTask.Outputs[outputID]
					fmt.Println("\tSuccessfully collected output from dependency task.")
				}
			}
		} else if strings.HasPrefix(source, parentTask.Root.ID) {
			// if the input source to this step is not the outputID of another step
			// but is an input of the parent workflow (e.g. "#subworkflow_test.cwl/input_bam" in ../testdata/workflow/workflow.json)
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
	task.Run()
}

// concurrently run steps of a workflow
func (task *Task) RunSteps() {
	// store a map of {outputID: stepID} pairs to trace step i/o dependency
	task.setupOutputMap()
	// NOTE: not sure if this should have a WaitGroup - seems to work fine without one
	for curStepID, subtask := range task.Children {
		go subtask.runStep(curStepID, task)
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
			stepID, ok := task.outputIDMap[source]
			if !ok {
				panic(fmt.Sprintf("Can't find output source %v", source))
			}
			subtaskOutputID := step2taskID(&task.Children[stepID].originalStep, source)
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
	task.outputIDMap = make(map[string]string)
	for _, step := range task.Root.Steps {
		for _, output := range step.Out {
			task.outputIDMap[output.ID] = step.ID
		}
	}
	return nil
}
