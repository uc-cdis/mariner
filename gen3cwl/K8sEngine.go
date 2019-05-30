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
		/*
			fmt.Println("Input:")
			PrintJSON(in)
			fmt.Println("Input.Provided:")
			PrintJSON(in.Provided)
		*/
	}
	// fmt.Println("OriginalStep:")
	// PrintJSON(tool.OriginalStep)
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
		NOTE: presently only context loaded into js vm's here is `self`
		Will certainly need to add more context to handle all cases
		Definitely, definitely need a generalized method for loading appropriate context at appropriate places
		In particular, the `inputs` context is probably going to be needed most commonly

		OTHERNOTE: `self` (in js vm) takes on different values in different places, according to cwl docs
		see: https://www.commonwl.org/v1.0/Workflow.html#Parameter_references
		---
		Steps:
		1. handle ValueFrom case at stepInput level
		 - if no ValueFrom specified, assign parameter value to `out` to processed in next step
		2. handle ValueFrom case at toolInput level
		 - initial value is `out` from step 1)
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
			// little evals like this need to happen all over the place in the cwl
			vm := otto.New()

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
	/*
		// Commenting out because the way commands are generated don't handle js expressions  gracefully..
		// See cwl.go/inputs.go/flatten() and Flatten() - this is used to generate commands for CLT's
		// hopefully we can still use this - but maybe need to write our own method to generate commands :/
			if input.Binding != nil && input.Binding.ValueFrom != nil {
				valueFrom := input.Binding.ValueFrom.Key()
				if strings.HasPrefix(valueFrom, "$") {
					vm := otto.New()
					vm.Set("self", out) // again, will more than likely need additional context here to cover other cases
					if out, err = EvalExpression(valueFrom, vm); err != nil {
						return nil, err
					}
				} else {
					// not an expression, so no eval necessary - take raw value
					out = valueFrom
				}
			}
	*/
	// fmt.Println("Here's tranformed input:")
	// PrintJSON(out)
	return out, nil
}

// loadInput passes input parameter value to input.Provided
func (tool *Tool) loadInput(input *cwl.Input) (err error) {
	// transformInput() handles any valueFrom statements at the workflowStepInput level and the tool input level
	// to be clear: "workflowStepInput level" refers to this tool and its inputs as they appear as a step in a workflow
	// so that would be specified in a cwl workflow file like Workflow.cwl
	// and the "tool input level" refers to the tool and its inputs as they appear in a standalone tool specification
	// so that information would be specified in a cwl  *tool file like CommandLineTool.cwl or ExpressionTool.cwl
	if provided, err := tool.transformInput(input); err == nil {
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	} else {
		fmt.Printf("error transforming input: %v\ninput: %v", err, input.ID)
		return err
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
// TODO: this cwl.go ToJavaScriptVM() function is super janky - need a better, more robust method of loading context
func (tool *Tool) inputsToVM() (err error) {
	prefix := tool.Root.ID + "/" // need to trim this from all the input.ID's
	tool.Root.InputsVM, err = tool.Root.Inputs.ToJavaScriptVM(prefix)
	if err != nil {
		fmt.Println("ERROR: failed to load js vm.")
		return err
	}
	return nil
}

// CollectOutput collects the output for a tool after the tool has run
// output parameter values get set, and the outputs parameter object gets stored in proc.Task.Outputs
// if the outputs of this process are the inputs of another process,
// then the output parameter object of this process (the Task.Outputs field)
// gets assigned as the input parameter object of that other process (the Task.Parameters field)
// ---
// may be a good idea to make different types for CLT and ExpressionTool
// and use Tool as an interface, so we wouldn't have to split cases like this
//  -> could just call one method in one line on a tool interface
// i.e., CollectOutput() should be a method on type CommandLineTool and on type ExpressionTool
// would bypass all this case-handling
// TODO: implement CommandLineTool and ExpressionTool types and their methods, as well as the Tool interface
// ---
// NOTE: the outputBinding for a given output parameter specifies how to assign a value to this parameter
// need to investigate/handle case when there is no outputBinding specified
// for ExpressionTool with a single output param with no binding,
// of course the single output value matches the single output parameter
// but outside of that ideal case -
// - if a CLT, or if multiple output values or multiple output parameters -
// how would output get collected? I feel this must be an error in the given cwl if this happens
func (proc *Process) CollectOutput() (err error) {
	proc.Task.Outputs = make(map[string]cwl.Parameter)
	switch class := proc.Tool.Root.Class; class {
	case "CommandLineTool":
		if err = proc.HandleCLTOutput(); err != nil {
			return err
		}
	case "ExpressionTool":
		if err = proc.HandleETOutput(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	return nil
}

// HandleCLTOutput assigns values to output parameters for this CommandLineTool
// stores resulting output parameters object in proc.Task.Outputs
// TODO
func (proc *Process) HandleCLTOutput() (err error) {
	for _, out := range proc.Task.Root.Outputs {
		fmt.Println("Here's an output parameter:")
		PrintJSON(out)
	}
	return nil
}

// HandleETOutput assigns values to output parameters for this ExpressionTool
// stores resulting output parameters object in proc.Task.Outputs
// will an ExpressionTool ever have more than one output parameter?
// TODO: investigate the case of multiple output parameters
// - find cwl examples and extend code to handle multiple ExpressionTool outputs (if necessary)
func (proc *Process) HandleETOutput() (err error) {
	switch n := len(proc.Task.Root.Outputs); n {
	case 0:
		// no outputs to collect
		return nil
	case 1:
		// ExpressionTool's expression result gets assigned to the only specified output parameter
		// this is the expected case
		proc.Task.Outputs[proc.Task.Root.Outputs[0].ID] = proc.Tool.ExpressionResult
		return nil
	default:
		// Presently not handling the case where there's more than one output specified for an ExpressionTool
		// Not sure if this is an expected/common case or not
		return fmt.Errorf("failed to handle more than one ExpressionTool output")
	}
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
		// fmt.Println("proc.Tool.Root.Outputs:")
		// PrintJSON(proc.Tool.Root.Outputs)
		proc.CollectOutput()
		// fmt.Println("ET task.Outputs after collecting outputs:")
		// PrintJSON(proc.Task.Outputs)

		// JS gets evaluated in-line, so the process is complete when the engine method RunExpressionTool() returns
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
	// note: context has already been loaded
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
	// fmt.Printf("\nExpression result type: %T\n", result)
	// fmt.Println("Expression result:")
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
