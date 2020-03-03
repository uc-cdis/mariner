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
	Parameters    cwl.Parameters         // input parameters of this task
	Root          *cwl.Root              // "root" of the "namespace" of the cwl file for this task
	Outputs       map[string]interface{} // output parameters of this task
	Scatter       []string               // if task is a step in a workflow and requires scatter; input parameters to scatter are stored here
	ScatterMethod string                 // if task is step in a workflow and requires scatter; scatter method specified - "dotproduct" or "flatcrossproduct" or ""
	ScatterTasks  map[int]*Task          // if task is a step in a workflow and requires scatter; scattered subtask objects stored here; scattered subtasks are enumerated
	ScatterIndex  int                    // if a task gets scattered, each subtask belonging to that task gets enumerated, and that index is stored here
	Children      map[string]*Task       // if task is a workflow; the Task objects of the workflow steps are stored here; {taskID: task} pairs
	OutputIDMap   map[string]string      // if task is a workflow; a map of {outputID: stepID} pairs in order to trace i/o dependencies between steps
	InputIDMap    map[string]string
	OriginalStep  *cwl.Step // if this task is a step in a workflow, this is the information from this task's step entry in the parent workflow's cwl file
	Done          *bool     // false until all output for this task has been collected, then true
	// --- New Fields ---
	Log           *Log           // contains Status, Stats, Event
	CleanupByStep *CleanupByStep // if task is a workflow; info for deleting intermediate files after they are no longer needed
}

// fileParam returns a bool indicating whether the given step-level input param corresponds to a set of files
// 'task' here is a workflow
// HERE
func (task *Task) stepParamFile(step *cwl.Step, stepParam string) bool {
	fmt.Println("in stepParamFile..")
	childTaskParam := step2taskID(step, stepParam)
	childTask := task.Children[step.ID]
	for _, input := range childTask.Root.Inputs {
		if input.ID == childTaskParam && inputParamFile(input) {
			return true
		}
	}
	return false
}

// for a given task input, return true if type File or []File, false otherwise
// NOTE: this structuring of type information is pretty painful to look at and deal with
// ----  could look into the cwl.go library again and maybe make it better
// ----  also not nice that Root.Inputs is an array rather than a map
// ----  could fix these things
func inputParamFile(input *cwl.Input) bool {
	fmt.Println("input.Types:")
	printJSON(input.Types)
	if input.Types[0].Type == "File" {
		return true
	}
	if input.Types[0].Type == "array" {
		for _, itemType := range input.Types[0].Items {
			if itemType.Type == "File" {
				return true
			}
		}
	}
	return false
}

// exact same function.. - NOTE: maybe implement method in cwl.go library instead of here
func outputParamFile(output cwl.Output) bool {
	fmt.Println("output.Types:")
	printJSON(output.Types)
	if output.Types[0].Type == "File" {
		return true
	}
	if output.Types[0].Type == "array" {
		for _, itemType := range output.Types[0].Items {
			if itemType.Type == "File" {
				return true
			}
		}
	}
	return false
}

// recursively populates `mainTask` (the task object for the top level workflow with all downstream task objects)
// see note describing the Task type for explanation of nested structure of Task
// basically, if task is a workflow, the task objects for the workflow steps get stored in the Task.Children field
// so the graph gets "resolved" via creating one big task (`mainTask`) which contains the entire workflow
// i.e., the whole workflow and its graphical structure are represented as a nested collection of Task objects
func (engine *K8sEngine) resolveGraph(rootMap map[string]*cwl.Root, curTask *Task) error {
	if curTask.Root.ID == mainProcessID {
		engine.Log.Main.Event.info("begin resolve graph")
	}
	if curTask.Root.Class == CWLWorkflow {
		curTask.Children = make(map[string]*Task)

		// serious "gotcha": https://medium.com/@betable/3-go-gotchas-590b8c014e0a
		/*
			"Go uses a copy of the value instead of the value itself within a range clause.
			So when we take the pointer of value, we’re actually taking the pointer of a copy
			of the value. This copy gets reused throughout the range clause [...]"
		*/
		for i, step := range curTask.Root.Steps {
			stepRoot, ok := rootMap[step.Run.Value]
			if !ok {
				return engine.errorf("failed to find workflow: %v", step.Run.Value)
			}

			newTask := &Task{
				Root:         stepRoot,
				Parameters:   make(cwl.Parameters),
				OriginalStep: &curTask.Root.Steps[i],
				Done:         &falseVal,
				Log:          logger(),
			}
			engine.Log.ByProcess[step.ID] = newTask.Log

			engine.resolveGraph(rootMap, newTask)

			curTask.Children[step.ID] = newTask
		}
	}
	if curTask.Root.ID == mainProcessID {
		engine.Log.Main.Event.info("end resolve graph")
	}
	return nil
}

// RunWorkflow parses a workflow and inputs and run it
func (engine *K8sEngine) runWorkflow() error {
	var root cwl.Root
	var err error
	var originalParams cwl.Parameters

	// Task object for top level workflow, later to be recursively populated
	// with task objects for all the other nodes in the workflow graph
	var mainTask *Task

	// unmarshal the packed workflow JSON from the request body
	if err = json.Unmarshal(engine.Log.Request.Workflow, &root); err != nil {
		// #log
		return err
	}

	// unmarshal the inputs JSON from the request body
	if err = json.Unmarshal(engine.Log.Request.Input, &originalParams); err != nil {
		// #log
		return err
	}

	// small preprocessing step to get the right input param IDs for the top level workflow
	params := make(cwl.Parameters)
	for id, value := range originalParams {
		params[fmt.Sprintf("%v/%v", mainProcessID, id)] = value
	}

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
		if process.ID == mainProcessID {
			// construct `mainTask` - the task object for the top level workflow
			mainTask = &Task{
				Root:       process,
				Parameters: params,
				Log:        logger(), // initialize empty Log object with status NOT_STARTED
			}
		}
	}
	if mainTask == nil {
		// #log
		return fmt.Errorf("can't find main workflow")
	}

	// fixme: refactor
	engine.Log.Main = mainTask.Log

	mainTask.Log.JobName = engine.Log.Request.JobName
	jobsClient, _, _, err := k8sClient(k8sJobAPI)
	if err != nil {
		return engine.errorf("%v", err)
	}
	mainTask.Log.JobID = engineJobID(jobsClient, engine.Log.Request.JobName)

	// recursively populate `mainTask` with Task objects for the rest of the nodes in the workflow graph
	if err = engine.resolveGraph(flatRoots, mainTask); err != nil {
		return engine.errorf("failed to resolve graph: %v", err)
	}

	// run the workflow
	if err = engine.run(mainTask); err != nil {
		return engine.errorf("failed to run main task: %v", err)
	}

	// not sure if this should be dangling here
	engine.Log.write()

	// delete all intermediate files generated by this workflow run
	// i.e., delete all paths under the workflow run working dir
	// ----- which are not paths associated with a main workflow output param
	engine.basicCleanup()

	/*
		// if running intermediate file cleanup as workflow is running
		// need some wait logic like this here to wait for all running cleanup processes to finish
		// before the engine job completes
		// wait for cleanup processes to finish before ending engine job
		fmt.Println("\tworkflow tasks finished; deleting intermediate files..")
		for len(engine.CleanupProcs) > 0 {
			fmt.Printf("\twaiting on %v cleanup processes to finish..\n", len(engine.CleanupProcs))
			printJSON(engine.CleanupProcs)
			time.Sleep(15 * time.Second) // same refresh period as the deleteCondition monitoring period
		}
		fmt.Println("\tall cleanup processes finished!")
	*/

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
// HERE - #log
func (engine *K8sEngine) run(task *Task) (err error) {
	fmt.Printf("\nRunning task: %v\n", task.Root.ID)
	engine.startTask(task)
	switch {
	case task.Scatter != nil:
		engine.runScatter(task)
		engine.gatherScatterOutputs(task) // Q. does this mean final log doesn't get written for scattered tasks?
	case task.Root.Class == "Workflow":
		// this is not a leaf in the graph
		fmt.Printf("Handling workflow %v..\n", task.Root.ID)
		engine.runSteps(task)
		if err = engine.mergeChildParams(task); err != nil {
			return engine.errorf("failed to merge child params for task: %v; error: %v", task.Root.ID, err)
		}
	default:
		// this is a leaf in the graph
		fmt.Printf("Dispatching task %v..\n", task.Root.ID)
		engine.dispatchTask(task)
	}
	engine.finishTask(task)
	return nil
}

func (engine *K8sEngine) mergeChildParams(task *Task) (err error) {
	if err = task.mergeChildOutputs(); err != nil {
		return task.Log.Event.errorf("failed to merge child outputs: %v", err)
	}

	// fixme: return and #log an error here
	task.mergeChildInputs() // for logging

	return nil
}

// fixme: return an error
func (task *Task) mergeChildInputs() {
	for _, child := range task.Children {
		for param := range child.Parameters {
			if wfParam, ok := task.InputIDMap[param]; ok {
				fmt.Println("collecting wf param ", wfParam)
				task.Log.Input[wfParam] = child.Log.Input[param]
			}
		}
	}
}

// FIXME - this function needs to be refactored - there's too much going on here
// ---- need to break it down into smaller parts
// ---- also should make these processes run concurrently
// ---- i.e., concurrently wait for each input parameter - not in sequence
// ---- because files should be deleted as soon as they become unnecessary
// for concurrent processing of steps of a workflow
// key point: the task does not get Run() until its input params are populated - that's how/where the dependencies get handled
func (engine *K8sEngine) runStep(curStepID string, parentTask *Task, task *Task) {
	fmt.Printf("\tProcessing Step: %v\n", curStepID)
	curStep := task.OriginalStep
	stepIDMap := make(map[string]string)
	fmt.Println("iterating through step inputs..")
	fmt.Println("curStep:")
	printJSON(curStep)
	for _, input := range curStep.In {
		fmt.Println("looking at input: ", input.ID)
		taskInput := step2taskID(curStep, input.ID)
		stepIDMap[input.ID] = taskInput // step input ID maps to [sub]task input ID

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

			// used for logging to merge child inputs for a workflow
			parentTask.InputIDMap[taskInput] = source
		}
	}

	// reaching here implies one of
	// 1. there are no step dependencies, or
	// 2. all step dependencies have been resolved/handled/run

	// if this step specifies input parameters to be scattered
	// collect those parameters in Task.Scatter array
	if len(curStep.Scatter) > 0 {
		for _, i := range curStep.Scatter {
			task.Scatter = append(task.Scatter, stepIDMap[i])
		}
	}

	// run this step
	engine.run(task)
}

// concurrently run steps of a workflow
func (engine *K8sEngine) runSteps(task *Task) {
	fmt.Println("\trunning steps..")

	// store a map of {outputID: stepID} pairs to trace step i/o dependency (edit: AND create CleanupByStep field)
	task.setupOutputMap()

	// dev'ing - not implementing for now
	// engine.cleanupByStep(task)

	task.InputIDMap = make(map[string]string)
	// NOTE: not sure if this should have a WaitGroup - seems to work fine without one
	for curStepID, subtask := range task.Children {
		go engine.runStep(curStepID, task, subtask)
	}
}

// "#expressiontool_test.cwl" + "[#subworkflow_test.cwl]/test_expr/file_array"
// returns "#expressiontool_test.cwl/test_expr/file_array"
func step2taskID(step *cwl.Step, stepParam string) string {
	return step.Run.Value + strings.TrimPrefix(stepParam, step.ID)
}

// only called if task is a workflow
// mergeChildOutputs maps outputs from the workflow's step tasks to the workflow task's output parameters
// i.e., task.Outputs is a map of (outputID, outputValue) pairs for all the outputs of this workflow
// where outputID is an output of the workflow AND an output of one of the steps of the workflow
// and outputValue is the value for that output parameter for the workflow step
// -> this outputValue gets mapped from the workflow step's outputs to the output of the workflow itself
func (task *Task) mergeChildOutputs() error {
	task.Outputs = make(map[string]interface{})
	if task.Children == nil {
		return task.Log.Event.errorf("failed to merge child outputs - no child tasks exist")
	}
	for _, output := range task.Root.Outputs {
		if len(output.Source) == 1 {
			// FIXME - again, here assuming len(source) is exactly 1
			source := output.Source[0]
			stepID, ok := task.OutputIDMap[source]
			if !ok {
				return task.Log.Event.errorf("failed to find output source: %v", source)
			}
			subtaskOutputID := step2taskID(task.Children[stepID].OriginalStep, source)
			fmt.Printf("Waiting to merge child outputs for workflow %v ..\n", task.Root.ID)
			for outputPresent := false; !outputPresent; _, outputPresent = task.Outputs[output.ID] {
				if outputVal, ok := task.Children[stepID].Outputs[subtaskOutputID]; ok {
					task.Outputs[output.ID] = outputVal
				}
			}
		} else {
			// fixme
			return task.Log.Event.error("NOT SUPPORTED: engine can't handle empty or array outputsource (this is a bug)")
		}
	}
	task.Log.Output = task.Outputs
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
		for _, stepOutput := range step.Out {
			task.OutputIDMap[stepOutput.ID] = step.ID
		}
	}
	return nil
}
