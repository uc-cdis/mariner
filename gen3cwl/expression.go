package gen3cwl

import (
	"fmt"

	cwl "github.com/uc-cdis/cwl.go"
)

// ExpressionTool represents a cwl expression tool
type ExpressionTool struct {
	Root       *cwl.Root
	Expression string
	Parameters cwl.Parameters
	Outputs    cwl.Parameters
}

// RunExpressionTool evaluates the js expression
// and stores the resulting task output parameter in exp.Outputs
func (exp *ExpressionTool) RunExpressionTool() error {
	fmt.Println("\tNeed to evaluate this expression:")
	fmt.Println(exp.Expression)
	return nil
}
