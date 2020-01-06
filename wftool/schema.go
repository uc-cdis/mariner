package wftool

import ()

// right now pretty much just writing out the CWL spec in Go types

// will this tool just marshal without enforcing/validating the cwl?
// e.g., if scatter, then scattermethod - will we perform that check here?
// or does this tool assume your cwl is error-free
// probably this tool should have some kind of validation function
// this tool should answer, to some degree, the question - "will this cwl run?"
// "will mariner even attempt to run this workflow?"

// WorkflowJSON is the JSON representation of a CWL workflow
type WorkflowJSON struct {
	Graph WorkflowGraph
}

// WorkflowGraph contains all the CWLObjects of the workflow
type WorkflowGraph []CWLObject

// as much as possible, don't repeat fields among structs
// make basic, atomic structs, and embed into other structs as needed

// CWLObject represents a workflow, expressiontool, commandlinetool, ...
// TODO
type CWLObject interface {
	// some methods
}

// ObjectMeta ..
type ObjectMeta struct {
	CoreMeta
	RequirementsAndHints
	Class      string
	CWLVersion string
}

// RequirementsAndHints ..
type RequirementsAndHints struct {
	Requirements []Requirement
	Hints        []Hint
}

// CoreMeta ..
type CoreMeta struct {
	ID    string
	Label string
	Doc   string
}

// Workflow ..
type Workflow struct {
	ObjectMeta
	Inputs  []InputParameter
	Outputs []WorkflowOutputParameter
	Steps   []WorkflowStep
}

// Run can be string | clt | workflow | expressiontool
// TODO
type Run interface{}

// WorkflowStep ..
type WorkflowStep struct {
	CoreMeta
	RequirementsAndHints
	In            []WorkflowStepInput
	Out           []WorkflowStepOutput
	Run           Run
	Scatter       []string
	ScatterMethod string
}

// WorkflowStepInput ..
// TODO
type WorkflowStepInput struct{}

// WorkflowStepOutput .. string or struct
// TODO
type WorkflowStepOutput interface{}

// InputParameter ..
// TODO
type InputParameter struct{}

// WorkflowOutputParameter ..
// TODO
type WorkflowOutputParameter struct{}

// CommandLineTool ..
type CommandLineTool struct {
	ObjectMeta
	Inputs             []CommandInputParameter
	Outputs            []CommandOutputParameter
	BaseCommand        []string
	Arguments          []Argument
	Stdin              string
	Stderr             string
	Stdout             string
	SuccessCodes       []int
	TemporaryFailCodes []int
	PermanentFailCodes []int
}

// Argument is one of 'expression' | 'string' | 'commandlinebinding'
// TODO
type Argument interface{}

// CommandInputParameter ..
// TODO
type CommandInputParameter struct{}

// CommandOutputParameter ..
// TODO
type CommandOutputParameter struct{}

// ExpressionTool ..
type ExpressionTool struct {
	ObjectMeta
	Inputs     []InputParameter
	Outputs    []ExpressionToolOutputParameter
	Expression string
}

// ExpressionToolOutputParameter ..
// TODO
type ExpressionToolOutputParameter struct{}

// Requirement ..
// TODO
type Requirement struct{}

// Hint ..
// TODO
type Hint struct{}
