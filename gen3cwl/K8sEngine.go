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
type K8sEngine struct {
	TaskSequence []string
	Commands     map[string][]string // for testing purposes
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
func (task *Task) getTool() Tool {
	tool := Tool{
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
func (tool *Tool) runTool() (err error) {
	fmt.Println("\tRunning tool..")
	if tool.Root.Expression != "" {
		err = tool.RunExpressionTool()
		if err != nil {
			return err
		}
	} else {
		err = tool.RunCommandLineTool()
		if err != nil {
			return err
		}
	}
	return nil
}

// RunCommandLineTool runs a commandline tool
func (tool *Tool) RunCommandLineTool() (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = tool.GenerateCommand()
	if err != nil {
		return err
	}
	err = tool.RunK8sJob()
	if err != nil {
		return err
	}
	return nil
}

// RunExpressionTool runs an ExpressionTool
func (tool *Tool) RunExpressionTool() (err error) {
	fmt.Println("\tRunning ExpressionTool..")
	err = tool.EvalExpression()
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
	js, fn, _ := GetJS(tool.Root.Expression) // strip the $() or ${}, which appears in the cwl as a wrapper for js expressions
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
			fmt.Printf("\tError running js function: %v\n", err)
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

// DispatchTask does some setup for and dispatches workflow *Tools - i.e., CommandLineTools and ExpressionTools
func (engine K8sEngine) DispatchTask(jobID string, task *Task) (err error) {

	////////////////////// temporarily loading output here for testing //////////////////
	switch task.Root.ID {
	case "#initdir_test.cwl":
		err := json.Unmarshal([]byte(`
			{"#initdir_test.cwl/bam_with_index": {
				"class": "File",
				"location": "NIST7035.1.chrM.bam",
				"secondaryFiles": [
					{
						"basename": "NIST7035.1.chrM.bam.bai",
						"location": "initdir_test.cwl/NIST7035.1.chrM.bam.bai",
						"class": "File"
					}
				]
			}}`), &task.Outputs)
		if err != nil {
			fmt.Printf("fail to unmarshal this thing\n")
		}
	case "#expressiontool_test.cwl":
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
			]}`), &task.Outputs)
		if err != nil {
			fmt.Printf("fail to unmarshal this thing\n")
		}
	}
	/////////////////////////////// temporarily loading output here for testing ///////////////

	tool := task.getTool()
	err = tool.loadInputs() // pass parameter values to input.Provided for each input (DONE)
	if err != nil {
		fmt.Printf("\tError loading inputs: %v\n", err)
		return err
	}
	err = tool.inputsToVM() // loads inputs context to js vm tool.Root.InputsVM (Ready to test, but needs to be extended)
	if err != nil {
		fmt.Printf("\tError loading inputs to js VM: %v\n", err)
		return err
	}
	err = tool.runTool() // runs the tool either as a CommandLineTool or ExpressionTool (DONE)
	if err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	return nil
}
