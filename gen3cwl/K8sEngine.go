package gen3cwl

// Engine defines specific implementation for an engine
type Engine interface {
	DispatchTask(jobID string, task *Task) error
	ListenOutputs(jobID string, task *Task) chan TaskResult
}

// K8sEngine uses k8s Job API to run workflows
type K8sEngine struct{}

// DispatchTask runs the tool as a docker container
func (K8sEngine) DispatchTask(jobID string, task *Task) error {
	// call k8s api to schedule job
	return nil
}

// ListenOutputs not implemented
func (K8sEngine) ListenOutputs(jobID string, task *Task) chan TaskResult {
	taskResult := make(chan TaskResult)
	taskResult <- TaskResult{}
	return taskResult
}
