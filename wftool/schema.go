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
	// some method
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
// TODO
type Workflow struct{}

// CommandLineTool ..
// TODO
type CommandLineTool struct{}

// ExpressionTool ..
// TODO
type ExpressionTool struct{}

// Requirement ..
// TODO
type Requirement struct{}

// Hint ..
// TODO
type Hint struct{}
