package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains the top level functions for the task engine
// the task engine
// 1. sets up a Tool
// 2. runs the Tool
// 3. if k8s job, then listens for job to finish

// K8sEngine runs all Tools, where a Tool is a CWL expressiontool or commandlinetool
// NOTE: engine object code store all the logs/event-monitoring/statistics for the workflow run
// ----- create some field, define a sensible data structure to easily collect/store/retreive logs
type K8sEngine struct {
	TaskSequence    []string               // for testing purposes
	UnfinishedProcs map[string]interface{} // engine's stack of CLT's that are running; (task.Root.ID, Process) pairs
	FinishedProcs   map[string]interface{} // engine's stack of completed processes; (task.Root.ID, Process) pairs
	UserID          string                 // the userID for the user who requested the workflow run
	RunID           string                 // the workflow timestamp
	Manifest        *Manifest              // to pass the manifest to the gen3fuse container of each task pod
	// ---- NEW FIELD ----
	Log *MainLog
}

// lots of room for better design here
// Tool interface
// CLT type
// ET type
// Workflow interface (?)
// Scatter interface (?)
//
// it should make sense! it should simplify things, not make things more complicated.

// Tool represents a leaf in the graph of a workflow
// i.e., a Tool is either a CommandLineTool or an ExpressionTool
// If Tool is a CommandLineTool, then it gets run as a k8s job in its own container
// When a k8s job gets created, a pointer to that Tool gets pushed onto the k8s engine's stack of UnfinishedProcs
// the k8s engine continuously iterates through the stack of running procs, retrieving job status from k8s api
// as soon as a job is complete, the pointer to the Tool gets popped from the stack
// and a function is called to collect the output from that Tool's completed process
//
// presently ExpressionTools run in a js vm in the mariner-engine, so they don't get dispatched as k8s jobs
type Tool struct {
	JobName          string // if a k8s job (i.e., if a CommandLineTool)
	JobID            string // if a k8s job (i.e., if a CommandLineTool)
	WorkingDir       string // e.g., /engine-workspace/workflowRuns/runID/taskID/
	Command          *exec.Cmd
	StepInputMap     map[string]*cwl.StepInput
	ExpressionResult map[string]interface{}
	Task             *Task
}

// Engine runs an instance of the mariner engine job
func Engine(runID string) error {
	request, err := request(runID)
	if err != nil {
		return err
	}
	engine := engine(request, runID)
	engine.runWorkflow(request.Workflow, request.Input)
	if err = done(runID); err != nil {
		return err
	}
	return nil
}

// get WorkflowRequest object
func request(runID string) (*WorkflowRequest, error) {
	request := &WorkflowRequest{}
	f, err := os.Open(fmt.Sprintf("/%v/workflowRuns/%v/request.json", ENGINE_WORKSPACE, runID))
	if err != nil {
		return request, err
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return request, err
	}
	err = json.Unmarshal(b, request)
	if err != nil {
		return request, err
	}
	return request, nil
}

// instantiate a K8sEngine object
// FIXME
func engine(request *WorkflowRequest, runID string) *K8sEngine {
	e := &K8sEngine{
		FinishedProcs:   make(map[string]interface{}),
		UnfinishedProcs: make(map[string]interface{}),
		Manifest:        &request.Manifest,
		UserID:          request.UserID,
		RunID:           runID,
	}

	// FIXME make this cleaner, less janky
	// HERE check if log already exists! if yes, then this is a 'restart'
	// definitely make this a constant, or combination of constants - not just hardcoded in-line like this
	pathToLog := fmt.Sprintf("/%v/workflowRuns/%v/marinerLog.json", ENGINE_WORKSPACE, runID)
	e.Log = mainLog(pathToLog, request)

	fmt.Println("in engine()")
	fmt.Println("here is engine log:")
	printJSON(e.Log)

	return e
}

// tell sidecar containers the workflow is done running so the engine job can finish
func done(runID string) error {
	if _, err := os.Create(fmt.Sprintf("/%v/workflowRuns/%v/done", ENGINE_WORKSPACE, runID)); err != nil {
		return err
	}
	return nil
}

// DispatchTask does some setup for and dispatches workflow Tools
func (engine K8sEngine) dispatchTask(task *Task) (err error) {
	tool := task.tool(engine.RunID)
	err = tool.setupTool()
	if err != nil {
		fmt.Printf("ERROR setting up tool: %v\n", err)
		return err
	}

	// {Q: when should the process get pushed onto the stack?}
	// push newly started process onto the engine's stack of running processes
	engine.UnfinishedProcs[tool.Task.Root.ID] = nil
	if err = engine.runTool(tool); err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	if err = engine.collectOutput(tool); err != nil {
		return err
	}
	engine.updateStack(tool)
	return nil
}

// move proc from unfinished to finished stack
func (engine *K8sEngine) updateStack(tool *Tool) {
	delete(engine.UnfinishedProcs, tool.Task.Root.ID)
	engine.FinishedProcs[tool.Task.Root.ID] = nil
	tool.Task.Done = &trueVal
}

// maybe this is a pointless wrapper?
func (engine *K8sEngine) collectOutput(tool *Tool) error {
	err := tool.collectOutput()
	return err
}

// GetTool returns a Tool object
// The Tool represents a workflow Tool and so is either a CommandLineTool or an ExpressionTool
// NOTE: tool looks like mostly a subset of task -> code needs to be polished/organized/refactored
func (task *Task) tool(runID string) *Tool {
	task.Outputs = make(cwl.Parameters)
	task.Log.Output = task.Outputs
	tool := &Tool{
		Task:       task,
		WorkingDir: task.workingDir(runID),
	}
	return tool
}

// see: https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html
// "characters to avoid" for keys in s3 buckets
// probably need to do some more filtering of other potentially problematic characters
// NOTE: should make the mount point a go constant - i.e., const MountPoint = "/engine-workspace/"
// ----- could come up with a better/more uniform naming scheme
func (task *Task) workingDir(runID string) string {
	safeID := strings.ReplaceAll(task.Root.ID, "#", "")
	dir := fmt.Sprintf("/%v/workflowRuns/%v/%v", ENGINE_WORKSPACE, runID, safeID)
	if task.ScatterIndex > 0 {
		dir = fmt.Sprintf("%v-scatter-%v", dir, task.ScatterIndex)
	}
	dir += "/"
	return dir
}

// create working directory for this Tool
func (tool *Tool) makeWorkingDir() error {
	err := os.MkdirAll(tool.WorkingDir, 0777)
	if err != nil {
		fmt.Printf("error while making directory: %v\n", err)
		return err
	}
	return nil
}

// performs some setup for a Tool to prepare for the engine to run the Tool
func (tool *Tool) setupTool() (err error) {
	err = tool.makeWorkingDir()
	if err != nil {
		return err
	}
	err = tool.loadInputs() // pass parameter values to input.Provided for each input
	if err != nil {
		fmt.Printf("\tError loading inputs: %v\n", err)
		return err
	}
	err = tool.inputsToVM() // loads inputs context to js vm tool.Task.Root.InputsVM (NOTE: Ready to test, but needs to be extended)
	if err != nil {
		fmt.Printf("\tError loading inputs to js VM: %v\n", err)
		return err
	}
	err = tool.initWorkDir()
	if err != nil {
		fmt.Println("Error handling initWorkDir req")
		return err
	}
	return nil
}

// RunTool runs the tool
// If ExpressionTool, passes to appropriate handler to eval the expression
// If CommandLineTool, passes to appropriate handler to create k8s job
func (engine *K8sEngine) runTool(tool *Tool) (err error) {
	switch class := tool.Task.Root.Class; class {
	case "ExpressionTool":
		if err = engine.runExpressionTool(tool); err != nil {
			return err
		}
	case "CommandLineTool":
		if err = engine.runCommandLineTool(tool); err != nil {
			return err
		}
		if err = engine.listenForDone(tool); err != nil {
			return fmt.Errorf("error listening for done: %v", err)
		}
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	return nil
}

// runCommandLineTool..
// 1. generates the command to execute
// 2. makes call to RunK8sJob to dispatch job to run the commandline tool
func (engine K8sEngine) runCommandLineTool(tool *Tool) (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = tool.generateCommand()
	if err != nil {
		return err
	}
	err = engine.dispatchTaskJob(tool)
	if err != nil {
		return err
	}
	return nil
}

// ListenForDone listens to k8s until the job status is "Completed"
// once that happens, calls a function to collect output and update engine's proc stacks
// TODO: implement error handling, listen for errors and failures, retries as well
// ----- handle the cases where the job status is not "Completed" or "Running"
func (engine *K8sEngine) listenForDone(tool *Tool) (err error) {
	fmt.Println("\tListening for job to finish..")
	status := ""
	for status != "Completed" {
		jobInfo, err := jobStatusByID(tool.JobID)
		if err != nil {
			return err
		}
		status = jobInfo.Status
	}
	return nil
}

func (engine *K8sEngine) runExpressionTool(tool *Tool) (err error) {
	// note: context has already been loaded
	if err = os.Chdir(tool.WorkingDir); err != nil {
		return err
	}
	result, err := evalExpression(tool.Task.Root.Expression, tool.Task.Root.InputsVM)
	if err != nil {
		return err
	}
	os.Chdir("/") // move back (?) to root after tool finishes execution -> or, where should the default directory position be?

	// expression must return a JSON object where the keys are the IDs of the ExpressionTool outputs
	// see description of `expression` field here:
	// https://www.commonwl.org/v1.0/Workflow.html#ExpressionTool
	var ok bool
	tool.ExpressionResult, ok = result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expressionTool expression did not return a JSON object")
	}
	return nil
}
