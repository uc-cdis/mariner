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
// 1. sets up a *Tool
// 2. runs the *Tool
// 3. if k8s job, then listens for job to finish

// K8sEngine runs all *Tools, where a *Tool is a CWL expressiontool or commandlinetool
// NOTE: engine object code store all the logs/event-monitoring/statistics for the workflow run
// ----- create some field, define a sensible data structure to easily collect/store/retreive logs
type K8sEngine struct {
	TaskSequence    []string            // for testing purposes
	UnfinishedProcs map[string]*Process // engine's stack of CLT's that are running; (task.Root.ID, Process) pairs
	FinishedProcs   map[string]*Process // engine's stack of completed processes; (task.Root.ID, Process) pairs
	UserID          string              // the userID for the user who requested the workflow run
	RunID           string              // the workflow timestamp
	Manifest        *Manifest           // to pass the manifest to the gen3fuse container of each task pod
	// ---- NEW FIELD ----
	Log *MainLog
}

// Process represents a leaf in the graph of a workflow
// i.e., a Process is either a CommandLineTool or an ExpressionTool
// If Process is a CommandLineTool, then it gets run as a k8s job in its own container
// When a k8s job gets created, a Process struct gets pushed onto the k8s engine's stack of UnfinishedProcs
// the k8s engine continuously iterates through the stack of running procs, retrieving job status from k8s api
// as soon as a job is complete, the Process struct gets popped from the stack
// and a function is called to collect the output from that completed process
//
// presently ExpressionTools run in a js vm in the mariner-engine, so they don't get dispatched as k8s jobs
type Process struct {
	JobName string // if a k8s job (i.e., if a CommandLineTool)
	JobID   string // if a k8s job (i.e., if a CommandLineTool)
	Tool    *Tool
	Task    *Task
}

// Tool represents a workflow *Tool - i.e., a CommandLineTool or an ExpressionTool
type Tool struct {
	Outdir           string // Given by context - NOTE: not sure what this is for
	WorkingDir       string // e.g., /engine-workspace/taskID/
	Root             *cwl.Root
	Parameters       cwl.Parameters
	Command          *exec.Cmd
	OriginalStep     cwl.Step
	StepInputMap     map[string]*cwl.StepInput // see: transformInput()
	ExpressionResult map[string]interface{}    // storing the result of an expression tool here for now - maybe there's a better way to do this
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
	fmt.Println("opening request..")
	f, err := os.Open(fmt.Sprintf("/%v/workflowRuns/%v/request.json", ENGINE_WORKSPACE, runID))
	if err != nil {
		return request, err
	}
	fmt.Println("reading request..")
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return request, err
	}
	fmt.Println("unmarshalling request..")
	err = json.Unmarshal(b, request)
	if err != nil {
		fmt.Printf("error: %v", err)
		return request, err
	}
	return request, nil
}

// instantiate a K8sEngine object
// FIXME
func engine(request *WorkflowRequest, runID string) *K8sEngine {
	e := &K8sEngine{
		FinishedProcs:   make(map[string]*Process),
		UnfinishedProcs: make(map[string]*Process),
		Manifest:        &request.Manifest,
		UserID:          request.ID,
		RunID:           runID,
	}

	// FIXME make this cleaner, less janky
	// HERE check if log already exists! if yes, then this is a 'restart'
	pathToLog := fmt.Sprintf("/%v/workflowRuns/%v/marinerLog.json", ENGINE_WORKSPACE, runID)
	e.Log = mainLog(pathToLog, request)

	return e
}

// tell sidecar containers the workflow is done running so the engine job can finish
func done(runID string) error {
	if _, err := os.Create(fmt.Sprintf("/%v/workflowRuns/%v/done", ENGINE_WORKSPACE, runID)); err != nil {
		return err
	}
	return nil
}

// DispatchTask does some setup for and dispatches workflow *Tools
func (engine K8sEngine) dispatchTask(task *Task) (err error) {
	tool := task.tool(engine.RunID)
	err = tool.setupTool()
	if err != nil {
		fmt.Printf("ERROR setting up tool: %v\n", err)
		return err
	}

	// NOTE: there's a lot of duplicated information here, because Tool is almost a subset of Task
	// this will be handled when code is refactored/polished/cleaned up

	// FIXME - refactor - either make a tool interface and have different types for expression vs. commandlinetool
	// or just put everything in the Task object (?)
	proc := &Process{
		Tool: tool,
		Task: task,
	}

	// {Q: when should the process get pushed onto the stack?}
	// push newly started process onto the engine's stack of running processes
	engine.UnfinishedProcs[tool.Root.ID] = proc
	if err = engine.runTool(proc); err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	if err = engine.collectOutput(proc); err != nil {
		return err
	}
	engine.updateStack(proc)
	return nil
}

// move proc from unfinished to finished stack
func (engine *K8sEngine) updateStack(proc *Process) {
	delete(engine.UnfinishedProcs, proc.Tool.Root.ID)
	engine.FinishedProcs[proc.Tool.Root.ID] = proc
	proc.Task.Done = &trueVal
}

func (engine *K8sEngine) collectOutput(proc *Process) error {
	err := proc.collectOutput()
	return err
}

// GetTool returns a Tool object
// The Tool represents a workflow *Tool and so is either a CommandLineTool or an ExpressionTool
// NOTE: tool looks like mostly a subset of task -> code needs to be polished/organized/refactored
func (task *Task) tool(runID string) *Tool {
	tool := &Tool{
		Root:         task.Root,
		Parameters:   task.Parameters,
		OriginalStep: task.originalStep,
		WorkingDir:   task.workingDir(runID),
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

// create working directory for this *Tool
func (tool *Tool) makeWorkingDir() error {
	err := os.MkdirAll(tool.WorkingDir, 0777)
	if err != nil {
		fmt.Printf("error while making directory: %v\n", err)
		return err
	}
	return nil
}

// performs some setup for a *Tool to prepare for the engine to run the *Tool
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
	err = tool.inputsToVM() // loads inputs context to js vm tool.Root.InputsVM (NOTE: Ready to test, but needs to be extended)
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
func (engine *K8sEngine) runTool(proc *Process) (err error) {
	switch class := proc.Tool.Root.Class; class {
	case "ExpressionTool":
		if err = engine.runExpressionTool(proc); err != nil {
			return err
		}
	case "CommandLineTool":
		if err = engine.runCommandLineTool(proc); err != nil {
			return err
		}
		if err = engine.listenForDone(proc); err != nil {
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
func (engine K8sEngine) runCommandLineTool(proc *Process) (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = proc.Tool.generateCommand()
	if err != nil {
		return err
	}
	err = engine.dispatchTaskJob(proc)
	if err != nil {
		return err
	}
	return nil
}

// ListenForDone listens to k8s until the job status is "Completed"
// once that happens, calls a function to collect output and update engine's proc stacks
// TODO: implement error handling, listen for errors and failures, retries as well
// ----- handle the cases where the job status is not "Completed" or "Running"
func (engine *K8sEngine) listenForDone(proc *Process) (err error) {
	fmt.Println("\tListening for job to finish..")
	status := ""
	for status != "Completed" {
		jobInfo, err := jobStatusByID(proc.JobID)
		if err != nil {
			return err
		}
		status = jobInfo.Status
	}
	return nil
}

func (engine *K8sEngine) runExpressionTool(proc *Process) (err error) {
	// note: context has already been loaded
	if err = os.Chdir(proc.Tool.WorkingDir); err != nil {
		return err
	}
	result, err := evalExpression(proc.Tool.Root.Expression, proc.Tool.Root.InputsVM)
	if err != nil {
		return err
	}
	os.Chdir("/") // move back (?) to root after tool finishes execution -> or, where should the default directory position be?

	// expression must return a JSON object where the keys are the IDs of the ExpressionTool outputs
	// see description of `expression` field here:
	// https://www.commonwl.org/v1.0/Workflow.html#ExpressionTool
	var ok bool
	proc.Tool.ExpressionResult, ok = result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expressionTool expression did not return a JSON object")
	}
	return nil
}
