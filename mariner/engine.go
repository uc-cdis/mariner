package mariner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains the top level functions for the task engine
// the task engine 1. sets up a *Tool and 2. runs the *Tool 3. if k8s job, then listens for job to finish

// initDirReq handles the InitialWorkDirRequirement if specified for this tool
// TODO: handle prefix issue; support cases where File or dirent is returned from `entry`
// NOTE: this function really needs to be cleaned up/revised
// ----- also this fn should go in a different file, maybe its own file
func (tool *Tool) initWorkDir() (err error) {
	var result, resFile interface{}

	for _, requirement := range tool.Root.Requirements {
		if requirement.Class == "InitialWorkDirRequirement" {
			fmt.Println("found InitialWorkDirRequirement:")
			PrintJSON(requirement)
			for _, listing := range requirement.Listing {
				// handling the case where `entry` is content (expression or string) to be written to a file
				// and `entryname` is the name of the file to be created
				var contents interface{}
				if strings.HasPrefix(listing.Entry, "$") {
					// `entry` is an expression which may return a string, File or `dirent`
					// NOTE: presently NOT supporting the File or dirent case
					// what's a dirent? good question: https://www.commonwl.org/v1.0/CommandLineTool.html#Dirent
					result, err = EvalExpression(listing.Entry, tool.Root.InputsVM)
					if err != nil {
						return err
					}
					fmt.Printf("entry expression: %v\n", listing.Entry)
					fmt.Println("result of entry expression:")
					PrintJSON(result)
					/*
						// to handle case where result is a file object
						// presently writing whatever the expression returns to the newly created file
						if isFile(result) {
							resFile = result
						} else {
							contents = result
						}
					*/
					contents = result
				} else {
					contents = listing.Entry
				}
				PrintJSON(contents)

				// `entryName` for sure is a string literal or an expression which evaluates to a string
				// `entryName` is the name of the file to be created
				var entryName string
				if strings.HasPrefix(listing.EntryName, "$") {
					result, err = EvalExpression(listing.EntryName, tool.Root.InputsVM)
					if err != nil {
						return err
					}
					var ok bool
					entryName, ok = result.(string)
					if !ok {
						return fmt.Errorf("entryname expression did not return a string")
					}
				} else {
					entryName = listing.EntryName
				}

				/*
					Cases:
					1. `entry` returned a file object - file object stored as an interface{} in `resFile` (NOT SUPPORTED)
					2. `entry` did not return a file object - then returned value is in `contents` and must be written to a new file with filename stored in `entryName` (supported)
				*/

				// HERE - TODO: handle same prefix issue as for collecting CLT output - see HandleCLTOutput()
				prefix := "/Users/mattgarvin/_fakes3/testWorkflow/#initdir_test.cwl" // for testing
				if resFile != nil {
					// "If the value is an expression that evaluates to a File object,
					// this indicates the referenced file should be added to the designated output directory prior to executing the tool."
					// NOTE: the "designated output directory" is just the directory corresponding to the *Tool
					// not sure what the purpose/meaning/use of this feature is - pretty sure all i/o for *Tools gets handled already
					// presently not supporting this case - will implement this feature once I find an example to work with
					panic("feature not supported: entry expression returned a file object")
				} else {
					jContents, err := json.Marshal(contents)
					if err != nil {
						return err
					}
					f, err := os.Create(filepath.Join(prefix, entryName))
					if err != nil {
						return err
					}
					f.Write(jContents)
					f.Close()
				}
			}
		}
	}
	return nil
}

// Engine ...
type Engine interface {
	DispatchTask(jobID string, task *Task) error
}

// K8sEngine runs all *Tools - including expression tools - should these functionalities be decoupled?
type K8sEngine struct {
	TaskSequence    []string            // for testing purposes
	Commands        map[string][]string // also for testing purposes
	UnfinishedProcs map[string]*Process // engine's stack of CLT's that are running (task.Root.ID, Process) pairs
	FinishedProcs   map[string]*Process // engine's stack of completed processes (task.Root.ID, Process) pairs
}

// Process represents a leaf in the graph of a workflow
// i.e., a Process is either a CommandLineTool or an ExpressionTool
// If Process is a CommandLineTool, then it gets run as a k8s job in its own container
// When a k8s job gets created, a Process struct gets pushed onto the k8s engine's stack of UnfinishedProcs
// the k8s engine continuously iterates through the stack of running procs, retrieving job status from k8s api
// as soon as a job is complete, the Process struct gets popped from the stack
// and a function is called to collect the output from that completed process
//
// presently ExpressionTools run in a js vm "in the workflow engine", so they don't get dispatched as k8s jobs
type Process struct {
	JobName string // if a k8s job (i.e., if a CommandLineTool)
	JobID   string // if a k8s job (i.e., if a CommandLineTool)
	Tool    *Tool
	Task    *Task
}

// Tool represents a workflow *Tool - i.e., a CommandLineTool or an ExpressionTool
type Tool struct {
	Outdir           string // Given by context
	Root             *cwl.Root
	Parameters       cwl.Parameters
	Command          *exec.Cmd
	OriginalStep     cwl.Step
	StepInputMap     map[string]*cwl.StepInput // see: transformInput()
	ExpressionResult map[string]interface{}    // storing the result of an expression tool here for now - maybe there's a better way to do this
}

// DispatchTask does some setup for and dispatches workflow *Tools - i.e., CommandLineTools and ExpressionTools
func (engine K8sEngine) DispatchTask(jobID string, task *Task) (err error) {
	tool := task.getTool()
	err = tool.setupTool()

	// NOTE: there's a lot of duplicated information here, because Tool is almost a subset of Task
	// this will be handled when code is refactored/polished/cleaned up
	proc := &Process{
		Tool: tool,
		Task: task,
	}

	// (when should the process get pushed onto the stack?)
	// push newly started process onto the engine's stack of running processes
	engine.UnfinishedProcs[tool.Root.ID] = proc

	// engine runs the tool either as a CommandLineTool or ExpressionTool
	err = engine.runTool(proc)
	if err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	return nil
}

// GetTool returns a Tool object
// The Tool represents a workflow *Tool and so is either a CommandLineTool or an ExpressionTool
// tool looks like mostly a subset of task..
// code needs to be polished/organized/refactored once the engine is actually running properly
func (task *Task) getTool() *Tool {
	tool := &Tool{
		Root:         task.Root,
		Parameters:   task.Parameters,
		OriginalStep: task.originalStep,
	}
	return tool
}

func (tool *Tool) setupTool() (err error) {
	err = tool.loadInputs() // pass parameter values to input.Provided for each input
	if err != nil {
		fmt.Printf("\tError loading inputs: %v\n", err)
		return err
	}
	err = tool.inputsToVM() // loads inputs context to js vm tool.Root.InputsVM (Ready to test, but needs to be extended)
	if err != nil {
		fmt.Printf("\tError loading inputs to js VM: %v\n", err)
		return err
	}
	err = tool.initWorkDir()
	if err != nil {
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
		err = engine.runExpressionTool(proc)
		if err != nil {
			return err
		}

		proc.CollectOutput()

		// JS gets evaluated in-line, so the process is complete when the engine method RunExpressionTool() returns
		delete(engine.UnfinishedProcs, proc.Tool.Root.ID)
		engine.FinishedProcs[proc.Tool.Root.ID] = proc

	case "CommandLineTool":
		err = engine.runCommandLineTool(proc)
		if err != nil {
			return err
		}
		err = engine.ListenForDone(proc) // tells engine to listen to k8s to check for this process to finish running
		if err != nil {
			return fmt.Errorf("error listening for done: %v", err)
		}
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	return nil
}

// RunCommandLineTool runs a CommandLineTool
func (engine K8sEngine) runCommandLineTool(proc *Process) (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = proc.Tool.GenerateCommand() // need to test different cases of generating commands
	if err != nil {
		return err
	}
	err = engine.RunK8sJob(proc) // push Process struct onto engine.UnfinishedProcs
	if err != nil {
		return err
	}
	return nil
}

// ListenForDone listens to k8s until the job status is "Completed"
// when complete, calls a function to collect output and update engine's proc stacks
func (engine *K8sEngine) ListenForDone(proc *Process) (err error) {
	fmt.Println("\tListening for job to finish..")
	status := ""
	for status != "Completed" {
		jobInfo, err := GetJobStatusByID(proc.JobID)
		if err != nil {
			return err
		}
		status = jobInfo.Status
	}
	fmt.Println("\tK8s job complete. Collecting output..")

	proc.CollectOutput()

	fmt.Println("\tFinished collecting output.")
	PrintJSON(proc.Task.Outputs)

	fmt.Println("\tUpdating engine process stack..")
	delete(engine.UnfinishedProcs, proc.Tool.Root.ID)
	engine.FinishedProcs[proc.Tool.Root.ID] = proc
	return nil
}

// RunExpressionTool runs an ExpressionTool
func (engine *K8sEngine) runExpressionTool(proc *Process) (err error) {
	// note: context has already been loaded
	result, err := EvalExpression(proc.Tool.Root.Expression, proc.Tool.Root.InputsVM)
	if err != nil {
		return err
	}

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
