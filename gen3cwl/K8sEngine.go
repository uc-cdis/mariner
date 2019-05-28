package gen3cwl

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

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
	ExpressionResult interface{} // storing the result of an expression tool here for now - maybe there's a better way to do this
}

// PrintJSON pretty prints a struct as json
func PrintJSON(i interface{}) {
	see, _ := json.MarshalIndent(i, "", "   ")
	fmt.Println(string(see))
}

// GetTool returns a Tool interface
// The Tool represents a workflow *Tool and so is either a CommandLineTool or an ExpressionTool
func (task *Task) getTool() *Tool {
	tool := &Tool{
		Root:       task.Root,
		Parameters: task.Parameters,
	}
	return tool
}

// LoadInputs passes parameter value to input.Provided for each input
func (tool *Tool) loadInputs() (err error) {
	sort.Sort(tool.Root.Inputs)
	for _, in := range tool.Root.Inputs {
		err = tool.loadInput(in)
		if err != nil {
			return err
		}
	}
	return nil
}

// loadInput passes input parameter value to input.Provided
func (tool *Tool) loadInput(input *cwl.Input) (err error) {
	if provided, ok := tool.Parameters[input.ID]; ok {
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

// LoadVM loads the js vm  with all the necessary variables
// to allow js expressions to be evaluated
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
	err = proc.Tool.EvalExpression()
	if err != nil {
		fmt.Printf("\tError during expression eval: %v\n", err)
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

// EvalExpression evaluates the expression of an ExpressionTool
// what should I do with the resulting value? - right now storing in tool.ExpressionResult
// this should be cleaned up
func (tool *Tool) EvalExpression() (err error) {
	js, fn, _ := GetJS(tool.Root.Expression) // strip the $() (or if ${} just trim leading $), which appears in the cwl as a wrapper for js expressions
	if js == "" {
		return fmt.Errorf("\tmissing expression")
	}
	/*
		fa, err := tool.Root.InputsVM.Run("inputs.file_array")
		fmt.Printf("\tHere's inputs.file_array: %v\n", fa)
		fmt.Printf("\tHere's the js:\n%v\n", js)
	*/
	if fn {
		// if expression wrapped like ${...}, need to run as a zero arg js function

		// construct js function definition
		fnDef := fmt.Sprintf("function f() %s", js)
		// fmt.Printf("Here's the fnDef:\n%v\n", fnDef)

		// run this function definition so the function exists in the vm
		tool.Root.InputsVM.Run(fnDef)

		// call this function in the vm
		if tool.ExpressionResult, err = tool.Root.InputsVM.Run("f()"); err != nil {
			fmt.Printf("\terror running js function: %v\n", err)
			return err
		}
	} else {
		if tool.ExpressionResult, err = tool.Root.InputsVM.Run(js); err != nil {
			return fmt.Errorf("\tfailed to evaluate js expression: %v", err)
		}
	}
	// HERE TODO
	// need to convert otto output value to a particular type
	// see output cwl def to determine what type to convert output to
	fmt.Printf("\tExpressionTool result: %T\n", tool.ExpressionResult)
	return nil
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
