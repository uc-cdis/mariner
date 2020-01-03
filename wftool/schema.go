package wftool

import ()

// WorkflowJSON is the JSON representation of a CWL workflow
type WorkflowJSON struct {
	Graph WorkflowGraph
}

// WorkflowGraph contains all the CWLObjects of the workflow
type WorkflowGraph []CWLObject

// CWLObject represents a workflow, expressiontool, commandlinetool, ...
type CWLObject interface {
	// some method
}

// CWLWorkflow ..
type CWLWorkflow struct{}

// CWLCommandLineTool ..
type CWLCommandLineTool struct{}

// CWLExpressionTool ..
type CWLExpressionTool struct{}
