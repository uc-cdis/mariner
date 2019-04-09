package gen3cwl

// TaskEngine defines a engine that runs task
type TaskEngine interface {
	RunCommandlineTool(tool *CommandLineTool) error
}

// K8sTaskEngine manages individual task
type K8sTaskEngine struct{}

// RunCommandlineTool constructs the command in sidecar, write the command to a script
// and wait for the actual pod to run the script, then gather the output
func (K8sTaskEngine) RunCommandlineTool(tool *CommandLineTool) error {
	tool.GenerateCommand()
	tool.GatherOutputs()
	return nil
}
