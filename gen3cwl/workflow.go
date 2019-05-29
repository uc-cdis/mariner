package gen3cwl

import (
	"encoding/json"
	"fmt"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// Task defines an instance of workflow/tool
type Task struct {
	JobID           string
	Engine          Engine
	Parameters      cwl.Parameters
	Root            *cwl.Root
	Outputs         chan TaskResult
	Scatter         []string
	ScatterMethod   string
	ScatterTasks    map[int]*Task
	Children        map[string]*Task
	unFinishedSteps map[string]struct{}
	outputIDMap     map[string]string
	originalStep    cwl.Step
}

// TaskResult captures result from a task
type TaskResult struct {
	Error   error
	Outputs cwl.Parameters
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
func RunWorkflow(jobID string, workflow []byte, inputs []byte, engine Engine) error {
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

	for _, workflow := range root.Graphs {
		flatRoots[workflow.ID] = workflow
		if workflow.ID == "#main" {
			mainTask = &Task{JobID: jobID, Root: workflow, Parameters: params, Engine: engine}
		}
	}
	if mainTask == nil {
		panic(fmt.Sprint("can't find main workflow"))
	}
	resolveGraph(flatRoots, mainTask)
	mainTask.Run()

	fmt.Print("\n")
	fmt.Print("finish")
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

func step2taskID(step *cwl.Step, stepVarID string) string {
	return step.Run.Value + strings.TrimPrefix(stepVarID, step.ID)
}

// megeChildOutputs maps outputs from children tasks to this task
func (task *Task) mergeChildOutputs() error {
	task.Outputs = make(chan TaskResult)
	go func() {
		result := TaskResult{}
		if task.Children == nil {
			result.Error = fmt.Errorf("can't call merge child outputs without childs %v", task.Root.ID)
			task.Outputs <- result
			return
		}
		for _, output := range task.Root.Outputs {
			if len(output.Source) == 1 {
				source := output.Source[0]
				stepID, ok := task.outputIDMap[source]
				if !ok {
					result.Error = fmt.Errorf("Can't find output source %v", source)
					task.Outputs <- result
					return
				}
				subtaskOutputID := step2taskID(&task.Children[stepID].originalStep, source)
				outputs := <-task.Children[stepID].Outputs
				if outputs.Error != nil {
					result.Error = fmt.Errorf("previous step %v failed", stepID)
					task.Outputs <- result
					return
				}
				outputVal, ok := outputs.Outputs[subtaskOutputID]
				if !ok {
					result.Error = fmt.Errorf("fail to get output from child step %v, %v", source, stepID)
					task.Outputs <- result
					return

				}
				result.Outputs[output.ID] = outputVal
			} else {
				panic(fmt.Sprintf("NOT SUPPORTED: don't know how to handle empty or array outputsource"))
			}

		}
		task.Outputs <- result
	}()

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
func (task *Task) Run() error {
	workflow := task.Root
	params := task.Parameters
	if task.Scatter != nil {
		task.runScatter()
		task.gatherScatterOutputs()
		return nil
	}
	if workflow.Class == "Workflow" {
		// create an unfinished steps map as a queue
		task.setupStepQueue()
		// store a map of outputID -> stepID to trace dependency
		task.setupOutputMap()

		var curStepID string
		var prevStepID string
		var curStep cwl.Step
		//  pick random step
		curStepID = task.getStep()

		// while there are unfinished steps
		for len(task.unFinishedSteps) > 0 {
			// resolve dependent steps
			prevStepID = ""

			subtask, ok := task.Children[curStepID]
			if !ok {
				panic(fmt.Sprintf("can't find workflow %v", curStepID))
			}
			curStep = subtask.originalStep

			idMaps := make(map[string]string)
			for _, input := range curStep.In {
				subtaskInput := step2taskID(&curStep, input.ID)
				idMaps[input.ID] = subtaskInput
				for _, source := range input.Source {
					// if source is an ID that points to an output in another step
					if stepID, ok := task.outputIDMap[source]; ok {
						if _, ok := task.unFinishedSteps[stepID]; ok {
							prevStepID = stepID
							break
						} else {
							depTask := task.Children[stepID]
							outputID := depTask.Root.ID + strings.TrimPrefix(source, stepID)
							depTaskResult := <-depTask.Outputs

							fmt.Printf("\nsource  %v \n", outputID)
							fmt.Printf("Previous task output %v \n", depTaskResult.Outputs)
							if depTaskResult.Error != nil {
								return fmt.Errorf(
									"failed to run task %v: %v",
									depTask.Root.ID, depTaskResult.Error.Error())
							}
							subtask.Parameters[subtaskInput] = depTaskResult.Outputs[outputID]
							fmt.Printf("inputID %v \n", subtaskInput)
							fmt.Printf("params %v \n", subtask.Parameters[subtaskInput])
						}
					} else if strings.HasPrefix(source, workflow.ID) {
						// step.in.id is composed of {stepID}/{inputID}
						// it's mappted to the step's workflow's input definition
						// which has the structure of {stepWorkflowID}/{inputID}
						subtask.Parameters[subtaskInput] = params[source]
						fmt.Printf("params %v \n", subtask.Parameters[subtaskInput])

					}

				}
				if prevStepID != "" {
					break
				}
			}

			// cancel processing this step, go to next loop to process dependent step
			if prevStepID != "" {
				curStepID = prevStepID
				fmt.Printf("go to dependent step %v \n", curStepID)
				continue
			}
			//  run step
			if len(curStep.Scatter) > 0 {
				// subtask.Scatter = make([]string, len(curStep.Scatter))
				for _, i := range curStep.Scatter {
					subtask.Scatter = append(subtask.Scatter, idMaps[i])
				}
			}
			fmt.Printf("run step %v \n", curStepID)
			subtask.Run()

			delete(task.unFinishedSteps, curStepID)
			// get random next step
			curStepID = task.getStep()
		}
		task.mergeChildOutputs()
	} else {
		if task.Scatter != nil {
			fmt.Printf("I am going to scatter this!!\n")
		}
		fmt.Printf("submit single job to k8s %v: %v \n", workflow.Class, workflow.ID)
		task.Engine.DispatchTask(task.JobID, task)
		task.Outputs = task.Engine.ListenOutputs(task.JobID, task)
		fmt.Print("\n")
	}
	// fmt.Print(workflow.Steps[0].ID)
	// fmt.Print(workflow.Steps[0].In[0].ValueFrom)
	// fmt.Print(workflow.Steps[0].In[0].Source)
	return nil
}
