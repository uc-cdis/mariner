package gen3cwl

import (
	"encoding/json"
	"fmt"
	"strings"
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

func (task *Task) gatherScatterOutputs() error {
	return nil
}

func (task *Task) runScatter() error {
	if task.ScatterMethod != "" && task.ScatterMethod != "dotproduct" {
		panic(fmt.Sprintf("NOT SUPPORTED scattermethod %v not supported", task.ScatterMethod))
	}
	task.ScatterTasks = make(map[int]*Task)
	// {"a": 1, "b": ["a", "b"]}
	// {"a": 1, "b": "a"}
	firstScatterKey := task.Scatter[0]
	castedParam := make(map[string][]interface{})
	for _, scatterKey := range task.Scatter {
		paramArray, ok := task.Parameters[scatterKey].([]interface{})
		if !ok {
			panic(fmt.Sprintf("Scatter on non-array input %v", scatterKey))
		}
		castedParam[scatterKey] = paramArray

	}

	for i := range castedParam[firstScatterKey] {
		subtask := &Task{
			JobID:      task.JobID,
			Engine:     task.Engine,
			Root:       task.Root,
			Parameters: make(cwl.Parameters),
		}
		for _, scatterKey := range task.Scatter {
			subtask.Parameters[scatterKey] = castedParam[scatterKey][i]
		}
		for k, v := range task.Parameters {
			if subtask.Parameters[k] != nil {
				subtask.Parameters[k] = v
			}
		}
		task.ScatterTasks[i] = subtask
		subtask.Run()
	}
	return nil
}

// Run a task
// a task is a process is a node on the graph
// a task can represent any of [Workflow, CommandLineTool, ExpressionTool, ...]
func (task *Task) Run() error {
	workflow := task.Root // use "process" instead of "workflow" as the variable name here
	params := task.Parameters

	fmt.Printf("\nRunning task: %v\n", workflow.ID)
	if task.Scatter != nil {
		/*
			task.runScatter()
			task.gatherScatterOutputs()
		*/
		fmt.Println("\tScatter not supported.")
		return nil
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
		task.Engine.DispatchTask(task.JobID, task)
	}
	return nil
}
