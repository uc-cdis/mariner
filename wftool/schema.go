package wftool

import ()

// right now pretty much just writing out the CWL spec in Go types

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

// CoreFields are common to workflow, expressiontool, commandlinetool, ...
type CoreFields struct {
	ObjectMeta
	ReqsAndHints
	Class      string
	CWLVersion string
}

// ReqsAndHints ..
type ReqsAndHints struct {
	Requirements []Requirement
	Hints        []Hint
}

// ObjectMeta ..
type ObjectMeta struct {
	ID    string
	Label string
	Doc   string
}

// Workflow ..
type Workflow struct {
	CoreFields
	Inputs  []InputParameter
	Outputs []WorkflowOutputParameter
	Steps   []WorkflowStep
}

// WorkflowStep ..
// TODO
type WorkflowStep struct{}

// InputParameter ..
// TODO
type InputParameter struct{}

// WorkflowOutputParameter ..
// TODO
type WorkflowOutputParameter struct{}

// CommandLineTool ..
type CommandLineTool struct {
	Inputs             []CommandInputParameter
	Outputs            []CommandOutputParameter
	BaseCommand        []string
	Arguments          []Argument
	Stdin              Expression
	Stderr             Expression
	Stdout             Expression
	SuccessCodes       []int
	TemporaryFailCodes []int
	PermanentFailCodes []int
}

// Expression is just a string - but making it explicit for clarity
type Expression string

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
	Inputs     []InputParameter
	Outputs    []ExpressionToolOutputParameter
	Expression Expression
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
