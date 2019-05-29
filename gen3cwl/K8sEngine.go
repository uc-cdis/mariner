package gen3cwl

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/robertkrimen/otto"
	cwl "github.com/uc-cdis/cwl.go"
)

// Engine ...
type Engine interface {
	DispatchTask(jobID string, task *Task) error
}

// K8sEngine uses k8s Job API to run workflows
// currently handles all *Tools - including expression tools - should these functionalities be decoupled?
type K8sEngine struct {
	TaskSequence    []string            // for testing purposes
	Commands        map[string][]string // also for testing purposes
	UnfinishedProcs map[string]*Process // engine's stack of CLT's that are running (task.Root.ID, Process) pairs
	FinishedProcs   map[string]*Process // engine's stack of completed processes (task.Root.ID, Process) pairs
	// JobsClient      JobInterface
}

// Process represents a leaf in the graph of a workflow
// i.e., a Process is either a CommandLineTool or an ExpressionTool
// If Process is a CommandLineTool, then it gets run as a k8s job in its own container
// When a k8s job gets created, a Process struct gets pushed onto the k8s engine's stack of UnfinishedProcs
// the k8s engine continously iterates through the stack of running procs, retrieving job status from k8s api
// as soon as a job is complete, the Process struct gets popped from the stack
// and a function is called to collect the output from that completed process
//
// presently ExpressionTools run in a js vm "in the workflow engine", so they don't get dispatched as k8s jobs
type Process struct {
	JobName string // if a k8s job (i.e., if a CommandLineTool)
	JobID   string // if a k8s job (i.e., if a CommandLineTool)
	Tool    *Tool
	Task    *Task
}

// Tool represents a workflow *Tool - i.e., a CommandLineTool or an ExpressionTool
type Tool struct {
	Outdir           string // Given by context
	Root             *cwl.Root
	Parameters       cwl.Parameters
	Command          *exec.Cmd
	OriginalStep     cwl.Step
	StepInputMap     map[string]*cwl.StepInput // see: transformInput()
	ExpressionResult interface{}               // storing the result of an expression tool here for now - maybe there's a better way to do this
}

// PrintJSON pretty prints a struct as json
func PrintJSON(i interface{}) {
	see, _ := json.MarshalIndent(i, "", "   ")
	fmt.Println(string(see))
}

// GetTool returns a Tool interface
// The Tool represents a workflow *Tool and so is either a CommandLineTool or an ExpressionTool
// tool looks like mostly a subset of task..
// code needs to be polished/organized/refactored once the engine is actually running properly
func (task *Task) getTool() *Tool {
	tool := &Tool{
		Root:         task.Root,
		Parameters:   task.Parameters,
		OriginalStep: task.originalStep,
	}
	return tool
}

// LoadInputs passes parameter value to input.Provided for each input
// TODO: Handle the "ValueFrom" case
// see: https://www.commonwl.org/user_guide/13-expressions/index.html
// in this setting, "ValueFrom" may appear either in:
//  - tool.Root.Inputs[i].inputBinding.ValueFrom, OR
//  - tool.OriginalStep.In[i].ValueFrom
// need to handle BOTH cases - first eval at the workflowStepInput level, then eval at the tool input level
func (tool *Tool) loadInputs() (err error) {
	sort.Sort(tool.Root.Inputs)
	tool.buildStepInputMap()
	for _, in := range tool.Root.Inputs {
		err = tool.loadInput(in)
		if err != nil {
			return err
		}
		fmt.Println("Input:")
		PrintJSON(in)
		fmt.Println("Input.Provided:")
		PrintJSON(in.Provided)
	}
	fmt.Println("OriginalStep:")
	PrintJSON(tool.OriginalStep)
	return nil
}

// used in loadInput() to handle case of workflow step input valueFrom case
func (tool *Tool) buildStepInputMap() {
	tool.StepInputMap = make(map[string]*cwl.StepInput)
	for _, in := range tool.OriginalStep.In {
		localID := GetLocalID(in.ID) // e.g., "file_array" instead of "#subworkflow_test.cwl/test_expr/file_array"
		tool.StepInputMap[localID] = &in
	}
}

// GetLocalID is a utility function. Example i/o:
// in: "#subworkflow_test.cwl/test_expr/file_array"
// out: "file_array"
func GetLocalID(s string) (localID string) {
	tmp := strings.Split(s, "/")
	return tmp[len(tmp)-1]
}

func (tool *Tool) transformInput(input *cwl.Input) (out interface{}, err error) {
	/*
		1. handle ValueFrom case at stepInput level (DONE - only context loaded is 'self')
		2. handle ValueFrom case at toolInput level (TODO)
		return resulting val
	*/
	localID := GetLocalID(input.ID)
	// stepInput ValueFrom case

	if tool.StepInputMap[localID].ValueFrom == "" {
		// no processing needs to happen if the valueFrom field is empty
		var ok bool
		if out, ok = tool.Parameters[input.ID]; !ok {
			return nil, fmt.Errorf("input not found in tool's parameters")
		}
	} else {
		// here the valueFrom field is not empty, so we need to eval valueFrom
		valueFrom := tool.StepInputMap[localID].ValueFrom
		if strings.HasPrefix(valueFrom, "$") {
			// valueFrom is an expression that needs to be eval'd
			// this block should be encapsulated into a function
			// little evals like this need to happen all over the place in the cwl
			// setting up, running, post-processing tools
			vm := otto.New()
			// right now only has "self" context - may need to add more context to handle all cases
			// definitely, definitely need a generalized method for loading appropriate context at appropriate places
			// in particular, the `inputs` context is probably going to be needed most commonly
			// `self` takes on different values in different places, according to cwl docs
			vm.Set("self", tool.Parameters[input.ID])
			if out, err = EvalExpression(valueFrom, vm); err != nil {
				return nil, err
			}
		} else {
			// valueFrom is not an expression - take raw string/val as value
			out = valueFrom
		}
	}
	// at this point, variable `out` is the transformed input thus far (even if no transformation actually occured)
	// so `out` will be what we work with in this next block as an initial value
	// tool inputBinding ValueFrom case
	if input.Binding != nil && input.Binding.ValueFrom != nil {
		// TODO
	}
	return out, nil
}

// loadInput passes input parameter value to input.Provided
func (tool *Tool) loadInput(input *cwl.Input) (err error) {
	/*
		if provided, ok := tool.Parameters[input.ID]; ok {
			input.Provided = cwl.Provided{}.New(input.ID, provided)
		}
	*/

	if provided, err := tool.transformInput(input); err != nil {
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	}

	if input.Default == nil && input.Binding == nil && input.Provided == nil {
		return fmt.Errorf("input `%s` doesn't have default field but not provided", input.ID)
	}
	if key, needed := input.Types[0].NeedRequirement(); needed {
		for _, req := range tool.Root.Requirements {
			for _, requiredtype := range req.Types {
				if requiredtype.Name == key {
					input.RequiredType = &requiredtype
					input.Requirements = tool.Root.Requirements
				}
			}
		}
	}
	return nil
}

// LoadVM loads tool.Root.InputsVM with inputs context - using Input.Provided for each input
// to allow js expressions to be evaluated
// TODO: pretty sure that not all the potentially necessary context gets loaded, presently
func (tool *Tool) inputsToVM() (err error) {
	prefix := tool.Root.ID + "/" // need to trim this from all the input.ID's
	tool.Root.InputsVM, err = tool.Root.Inputs.ToJavaScriptVM(prefix)
	if err != nil {
		fmt.Println("ERROR: failed to load js vm.")
		return err
	}
	return nil
}

// RunTool runs the tool
// If ExpressionTool, passes to appropriate handler to eval the expression
// If CommandLineTool, passes to appropriate handler to create k8s job
func (engine *K8sEngine) runTool(proc *Process) (err error) {
	fmt.Println("\tRunning tool..")
	switch class := proc.Tool.Root.Class; class {
	case "ExpressionTool":
		err = engine.RunExpressionTool(proc)
		if err != nil {
			return err
		}
		// JS gets evaluated in-line, so the process is complete when the engine method RunExpressionTool() returns
		// NEED to collect output somewhere 'round here
		// temporarily hardcoding output here for testing
		err := json.Unmarshal([]byte(`
						{"#expressiontool_test.cwl/output": [
							{"bam_with_index": {
								"class": "File",
								"location": "NIST7035.1.chrM.bam",
								"secondaryFiles": [
									{
										"basename": "NIST7035.1.chrM.bam.bai",
										"location": "initdir_test.cwl/NIST7035.1.chrM.bam.bai",
										"class": "File"
									}
								]
							}}
						]}`), &proc.Task.Outputs)
		if err != nil {
			fmt.Printf("fail to unmarshal this thing\n")
		}
		delete(engine.UnfinishedProcs, proc.Tool.Root.ID)
		engine.FinishedProcs[proc.Tool.Root.ID] = proc
	case "CommandLineTool":
		err = engine.RunCommandLineTool(proc)
		if err != nil {
			return err
		}
		err = engine.ListenForDone(proc) // tells engine to listen to k8s to check for this process to finish running
		if err != nil {
			return fmt.Errorf("error listening for done: %v", err)
		}
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	return nil
}

// RunCommandLineTool runs a CommandLineTool
func (engine K8sEngine) RunCommandLineTool(proc *Process) (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = proc.Tool.GenerateCommand() // this should happen in the task engine - how exactly does that happen?
	if err != nil {
		return err
	}
	err = engine.RunK8sJob(proc) // push Process struct onto engine.UnfinishedProcs
	if err != nil {
		return err
	}
	return nil
}

// RunExpressionTool runs an ExpressionTool
func (engine *K8sEngine) RunExpressionTool(proc *Process) (err error) {
	fmt.Println("\tRunning ExpressionTool..")
	proc.Tool.ExpressionResult, err = EvalExpression(proc.Tool.Root.Expression, proc.Tool.Root.InputsVM)
	if err != nil {
		return err
	}
	return nil
}

// GetJS strips the cwl prefix for an expression
// and tells whether to just eval the expression, or eval the exp as a js function
// this is modified from the cwl.Eval.ToJavaScriptString() method
func GetJS(s string) (js string, fn bool, err error) {
	// if curly braces, then need to eval as a js function
	// see https://www.commonwl.org/v1.0/Workflow.html#Expressions
	fn = strings.HasPrefix(s, "${")
	s = strings.TrimLeft(s, "$(\n")
	// s = regexp.MustCompile("\\)$").ReplaceAllString(s, "")
	s = strings.TrimRight(s, ")\n")
	// fmt.Printf("\tHere's the js: %v\n", s)
	return s, fn, nil
}

// EvalExpression is an engine for handling in-line js in cwl
// the exp is passed before being stripped of any $(...) or ${...} wrapper
// the vm must be loaded with all necessary context for eval
// EvalExpression handles parameter references and expressions $(...), as well as functions ${...}
func EvalExpression(exp string, vm *otto.Otto) (result interface{}, err error) {
	// strip the $() (or if ${} just trim leading $), which appears in the cwl as a wrapper for js expressions
	var output otto.Value
	js, fn, _ := GetJS(exp)
	if js == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if fn {
		// if expression wrapped like ${...}, need to run as a zero arg js function

		// construct js function definition
		fnDef := fmt.Sprintf("function f() %s", js)
		// fmt.Printf("Here's the fnDef:\n%v\n", fnDef)

		// run this function definition so the function exists in the vm
		vm.Run(fnDef)

		// call this function in the vm
		output, err = vm.Run("f()")
		if err != nil {
			fmt.Printf("\terror running js function: %v\n", err)
			return nil, err
		}
	} else {
		output, err = vm.Run(js)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate js expression: %v", err)
		}
	}
	result, _ = output.Export()
	// fmt.Printf("\nExpression result type: %T", result)
	// PrintJSON(result)
	return result, nil
}

func (tool *Tool) setupTool() (err error) {
	err = tool.loadInputs() // pass parameter values to input.Provided for each input
	if err != nil {
		fmt.Printf("\tError loading inputs: %v\n", err)
		return err
	}
	err = tool.inputsToVM() // loads inputs context to js vm tool.Root.InputsVM (Ready to test, but needs to be extended)
	if err != nil {
		fmt.Printf("\tError loading inputs to js VM: %v\n", err)
		return err
	}
	return nil
}

// DispatchTask does some setup for and dispatches workflow *Tools - i.e., CommandLineTools and ExpressionTools
func (engine K8sEngine) DispatchTask(jobID string, task *Task) (err error) {
	tool := task.getTool()
	err = tool.setupTool()
	//  when should the process get pushed onto the stack?
	// also, there's a lot of duplicated information here, because Tool is almost a subset of Task
	// this will be handled when code is refactored/polished/cleaned up
	proc := &Process{
		Tool: tool,
		Task: task,
	}
	engine.UnfinishedProcs[tool.Root.ID] = proc // push newly started process onto the engine's stack of running processes
	err = engine.runTool(proc)                  // engine runs the tool either as a CommandLineTool or ExpressionTool

	if err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	return nil
}
