package mariner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/robertkrimen/otto"
	log "github.com/sirupsen/logrus"
	cwl "github.com/uc-cdis/cwl.go"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	sync.RWMutex    `json:"-"`
	S3FileManager   *S3FileManager
	TaskSequence    []string            // for testing purposes
	UnfinishedProcs map[string]bool     // engine's stack of CLT's that are running; (task.Root.ID, Process) pairs
	FinishedProcs   map[string]bool     // engine's stack of completed processes; (task.Root.ID, Process) pairs
	CleanupProcs    map[CleanupKey]bool // engine's stack of running cleanup processes
	UserID          string              // the userID for the user who requested the workflow run
	RunID           string              // the workflow timestamp
	Manifest        *Manifest           // to pass the manifest to the gen3fuse container of each task pod
	Log             *MainLog            //
	KeepFiles       map[string]bool     // all the paths to not delete during basic file cleanup
	IsInitWorkDir   string
}

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
	WorkingDir       string
	Command          *exec.Cmd
	StepInputMap     map[string]*cwl.StepInput
	ExpressionResult map[string]interface{}
	Task             *Task
	S3Input          *ToolS3Input
	initWorkDirFiles []string
	commonsUID       []string

	// dev'ing
	// need to load this with runtime context as per CWL spec
	// https://www.commonwl.org/v1.0/CommandLineTool.html#Runtime_environment
	// for now, only populating 'runtime.outdir'
	JSVM     *otto.Otto
	InputsVM *otto.Otto
}

// TaskRuntimeJSContext gets loaded into the js vm
// to allow in-line js expressions and parameter references in the CWL to be resolved
// see: https://www.commonwl.org/v1.0/CommandLineTool.html#Runtime_environment
//
// NOTE: not currently supported: tmpdir, cores, ram, outdirSize, tmpdirSize
type TaskRuntimeJSContext struct {
	Outdir string `json:"outdir"`
}

// ToolS3Input ..
type ToolS3Input struct {
	Paths []string `json:"paths"`
}

// Engine runs an instance of the mariner engine job
func Engine(runID string) (err error) {
	engine := engine(runID)

	defer func() {
		if r := recover(); r != nil {
			engine.Log.Main.Status = failed
			err = engine.errorf("mariner panicked: %v", r)
		}
	}()

	if err = engine.loadRequest(); err != nil {
		return engine.errorf("failed to load workflow request: %v", err)
	}
	if err = engine.runWorkflow(); err != nil {
		return engine.errorf("failed to run workflow: %v", err)
	}

	// turning off file cleanup because it's busted and must be fixed
	// engine.basicCleanup()

	return err
}

// get WorkflowRequestJSON from the run working directory in S3
//
// location of request:
// s3://workflow-engine-garvin/$USER_ID/workflowRuns/$RUN_ID/request.json
// key is "/$USER_ID/workflowRuns/$RUN_ID/request.json"
// key format is "/%s/workflowRuns/%s/%s"
//
// key := fmt.Sprintf("/%s/workflowRuns/%s/%s", engine.UserID, engine.RunID, requestFile)
func (engine *K8sEngine) fetchRequestFromS3() (*WorkflowRequest, error) {
	sess := engine.S3FileManager.newS3Session()
	downloader := s3manager.NewDownloader(sess)
	buf := &aws.WriteAtBuffer{}

	key := fmt.Sprintf("/%s/workflowRuns/%s/%s", engine.UserID, engine.RunID, requestFile)

	s3Obj := &s3.GetObjectInput{
		Bucket: aws.String(engine.S3FileManager.S3BucketName),
		Key:    aws.String(key),
	}

	_, err := downloader.Download(buf, s3Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to download file, %v", err)
	}

	b := buf.Bytes()
	r := &WorkflowRequest{}
	err = json.Unmarshal(b, r)
	if err != nil {
		return nil, fmt.Errorf("error unmarhsalling TaskS3Input: %v", err)
	}
	return r, nil
}

// instantiate a K8sEngine object
func engine(runID string) *K8sEngine {
	e := &K8sEngine{
		FinishedProcs:   make(map[string]bool),
		UnfinishedProcs: make(map[string]bool),
		CleanupProcs:    make(map[CleanupKey]bool),
		RunID:           runID,
		UserID:          os.Getenv(userIDEnvVar),
		Log:             mainLog(fmt.Sprintf(pathToLogf, runID)),
	}

	fm := &S3FileManager{}

	if err := fm.setup(); err != nil {
		log.Error("FAILED TO SETUP S3FILEMANAGER")
	}
	e.S3FileManager = fm
	return e
}

func (engine *K8sEngine) loadRequest() error {
	engine.infof("begin load workflow request")
	request, err := engine.fetchRequestFromS3()
	if err != nil {
		return engine.errorf("failed to load workflow request: %v", err)
	}
	engine.Manifest = &request.Manifest
	engine.Log.Request = request
	engine.infof("end load workflow request")
	return nil
}

// DispatchTask does some setup for and dispatches workflow Tools
func (engine *K8sEngine) dispatchTask(task *Task) (err error) {
	engine.infof("begin dispatch task: %v", task.Root.ID)

	engine.Lock()
	tool := task.tool(engine.RunID) // #race #ok
	engine.Unlock()

	if err = engine.setupTool(tool); err != nil {
		return engine.errorf("failed to setup tool: %v; error: %v", task.Root.ID, err)
	}
	if err = engine.runTool(tool); err != nil {
		return engine.errorf("failed to run tool: %v; error: %v", task.Root.ID, err)
	}
	if err = engine.collectOutput(tool); err != nil {
		return engine.errorf("failed to collect output for tool: %v; error: %v", task.Root.ID, err)
	}
	if err = engine.deletePVC(tool); err != nil {
		engine.warnf("failed to delete pvc for tool: %v", task.Root.ID)
	}
	engine.infof("end dispatch task: %v", task.Root.ID)
	return nil
}

func (engine *K8sEngine) deletePVC(tool *Tool) error {
	claimName := fmt.Sprintf("%s-claim", tool.JobName)
	coreClient, _, _, _, err := k8sClient(k8sCoreAPI)
	if err != nil {
		return err
	}
	err = coreClient.PersistentVolumeClaims(os.Getenv("GEN3_NAMESPACE")).Delete(context.TODO(), claimName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

// move proc from unfinished to finished stack
func (engine *K8sEngine) finishTask(task *Task) {
	engine.Lock()
	defer engine.Unlock()

	delete(engine.UnfinishedProcs, task.Root.ID)
	engine.FinishedProcs[task.Root.ID] = true
	engine.finishTaskLog(task)

	// task.Lock()
	task.Done = &trueVal // #race #ok
	// task.Unlock()
}

// push newly started process onto the engine's stack of running processes
// initialize log
func (engine *K8sEngine) startTask(task *Task) {
	engine.Lock()
	engine.UnfinishedProcs[task.Root.ID] = true // #race #ok
	engine.Unlock()

	engine.startTaskLog(task)
}

// collectOutput collects the output for a tool after the tool has run
// output parameter values get set, and the outputs parameter object gets stored in tool.Task.Outputs
// if the outputs of this process are the inputs of another process,
// then the output parameter object of this process (the Task.Outputs field)
// gets assigned as the input parameter object of that other process (the Task.Parameters field)
// ---
// may be a good idea to make different types for CLT and ExpressionTool
// and use Tool as an interface, so we wouldn't have to split cases like this
//  -> could just call one method in one line on a tool interface
// i.e., CollectOutput() should be a method on type CommandLineTool and on type ExpressionTool
// would bypass all this case-handling
// TODO: implement CommandLineTool and ExpressionTool types and their methods, as well as the Tool interface
// ---
// NOTE: the outputBinding for a given output parameter specifies how to assign a value to this parameter
// ----- no binding provided -> output won't be collected
func (engine *K8sEngine) collectOutput(tool *Tool) (err error) {
	engine.infof("begin collect output for task: %v", tool.Task.Root.ID)
	tool.Task.infof("begin collect output")
	switch class := tool.Task.Root.Class; class {
	case CWLCommandLineTool:
		if err = engine.handleCLTOutput(tool); err != nil {
			return tool.Task.errorf("%v", err)
		}
	case CWLExpressionTool:
		if err = engine.handleETOutput(tool); err != nil {
			return tool.Task.errorf("%v", err)
		}
	default:
		return tool.Task.errorf("unexpected class: %v", class)
	}
	tool.Task.infof("end collect output")
	engine.infof("end collect output for task: %v", tool.Task.Root.ID)
	return nil
}

// The Tool represents a workflow Tool and so is either a CommandLineTool or an ExpressionTool
func (task *Task) tool(runID string) *Tool {
	task.infof("begin make tool object")
	task.Outputs = make(map[string]interface{}) // #race #ok
	task.Log.Output = task.Outputs              // #race #ok
	tool := &Tool{
		Task:       task,
		WorkingDir: task.workingDir(runID),
		S3Input: &ToolS3Input{
			Paths: []string{},
		},
	}
	tool.JSVM = tool.newJSVM()
	task.infof("end make tool object")
	return tool
}

// should be called exactly once - when a tool is created in the first place
// all other vm's created should be copied from this one
// dev'ing
func (tool *Tool) newJSVM() *otto.Otto {
	vm := otto.New()
	runtime := &TaskRuntimeJSContext{Outdir: tool.WorkingDir}
	/*
		ctx := struct {
			Runtime TaskRuntimeJSContext `json:"runtime"`
		}{
			*runtime,
		}
	*/
	// runtimeJSVal, err := preProcessContext(ctx)
	runtimeJSVal, err := preProcessContext(runtime)
	if err != nil {
		panic(fmt.Errorf("failed to preprocess runtime js context: %v", err))
	}
	vm.Set("runtime", runtimeJSVal)
	return vm
}

// see: https://docs.aws.amazon.com/AmazonS3/latest/dev/UsingMetadata.html
// "characters to avoid" for keys in s3 buckets
// probably need to do some more filtering of other potentially problematic characters
// NOTE: should make the mount point a go constant - i.e., const MountPoint = "/engine-workspace/"
// ----- could come up with a better/more uniform naming scheme
func (task *Task) workingDir(runID string) string {
	task.infof("begin make task working dir")

	safeID := strings.ReplaceAll(task.Root.ID, "#", "")

	// task.Root.ID is not unique among tool runs
	// so, adding this random 4 char suffix
	// this is not really a perfect solution
	// because errors can still happen, though with very low probability
	// "error" here meaning by chance creating a `safeID` that's already been used
	// --- by a previous run of this same tool/task object
	safeID = fmt.Sprintf("%v-%v", safeID, getRandString(4))

	dir := fmt.Sprintf(pathToWorkingDirf, runID, safeID)
	if task.ScatterIndex > 0 {
		dir = fmt.Sprintf("%v-scatter-%v", dir, task.ScatterIndex)
	}
	dir += "/"
	task.infof("end make task working dir: %v", dir)
	return dir
}

func (engine *K8sEngine) writeFileInputListToS3(tool *Tool) error {
	tool.Task.infof("being write file input list to s3")
	sess := engine.S3FileManager.newS3Session()
	uploader := s3manager.NewUploader(sess)

	key := filepath.Join(engine.S3FileManager.s3Key(tool.WorkingDir, engine.UserID), inputFileListName)

	b, err := json.Marshal(tool.S3Input)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}

	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(engine.S3FileManager.S3BucketName),
		Key:    aws.String(key),
		Body:   bytes.NewReader(b),
	})
	if err != nil {
		return fmt.Errorf("failed to upload file list to s3")
	}
	log.Info("wrote input file list to s3 location:", result.Location)
	tool.Task.infof("end write file input list to s3")
	return nil
}

// performs some setup for a Tool to prepare for the engine to run the Tool
func (engine *K8sEngine) setupTool(tool *Tool) (err error) {
	tool.Task.infof("begin setup tool")

	// pass parameter values to input.Provided for each input
	if err = engine.loadInputs(tool); err != nil {
		return tool.Task.errorf("failed to load inputs: %v", err)
	}

	// loads inputs context to js vm tool.InputsVM (NOTE: Ready to test, but needs to be extended)
	if err = tool.inputsToVM(); err != nil {
		return tool.Task.errorf("failed to load inputs to js vm: %v", err)
	}

	if err = engine.initWorkDirReq(tool); err != nil {
		return tool.Task.errorf("failed to handle initWorkDir requirement: %v", err)
	}

	// write list of input files to tool "working directory" in S3
	if err = engine.writeFileInputListToS3(tool); err != nil {
		return tool.Task.errorf("failed to write file input list to s3: %v", err)
	}

	tool.Task.infof("end setup tool")
	return nil
}

// RunTool runs the tool from the engine and passes to the appropriate handler to create a k8s job.
func (engine *K8sEngine) runTool(tool *Tool) (err error) {
	engine.infof("begin run tool: %v", tool.Task.Root.ID)
	switch class := tool.Task.Root.Class; class {
	case "ExpressionTool":
		if err = engine.runExpressionTool(tool); err != nil {
			return engine.errorf("failed to run ExpressionTool: %v; error: %v", tool.Task.Root.ID, err)
		}
		if err = engine.listenForDone(tool); err != nil {
			return engine.errorf("failed to listen for task to finish: %v; error: %v", tool.Task.Root.ID, err)
		}
	case "CommandLineTool":
		if err = engine.runCommandLineTool(tool); err != nil {
			return engine.errorf("failed to run CommandLineTool: %v; error: %v", tool.Task.Root.ID, err)
		}
		go engine.collectResourceMetrics(tool)
		if err = engine.listenForDone(tool); err != nil {
			return engine.errorf("failed to listen for task to finish: %v; error: %v", tool.Task.Root.ID, err)
		}
	default:
		return engine.errorf("failed to run CWL object of unexpected class: %v", class)
	}
	engine.infof("end run tool: %v", tool.Task.Root.ID)
	return nil
}

// runCommandLineTool..
// 1. generates the command to execute
// 2. makes call to RunK8sJob to dispatch job to run the commandline tool
func (engine *K8sEngine) runCommandLineTool(tool *Tool) (err error) {
	engine.infof("begin run CommandLineTool: %v", tool.Task.Root.ID)
	err = tool.generateCommand()
	if err != nil {
		return engine.errorf("failed to generate command for tool: %v; error: %v", tool.Task.Root.ID, err)
	}
	err = engine.dispatchTaskJob(tool)
	if err != nil {
		return engine.errorf("failed to dispatch task job: %v; error: %v", tool.Task.Root.ID, err)
	}
	engine.infof("end run CommandLineTool: %v", tool.Task.Root.ID)
	return nil
}

// ListenForDone listens to k8s until the job status is COMPLETED
// once that happens, calls a function to collect output and update engine's proc stacks
// TODO: implement error handling, listen for errors and failures, retries as well
// ----- handle the cases where the job status is not COMPLETED or RUNNING
func (engine *K8sEngine) listenForDone(tool *Tool) (err error) {
	engine.infof("begin listen for task to finish: %v", tool.Task.Root.ID)
	status := ""
	for status != completed {
		jobInfo, err := jobStatusByID(tool.JobID)
		if err != nil {
			return engine.errorf("failed to get task job info: %v; error: %v", tool.Task.Root.ID, err)
		}
		status = jobInfo.Status
	}
	engine.infof("end listen for task to finish: %v", tool.Task.Root.ID)
	return nil
}

// runExpressionTool uses the engine to dispatch a task job for a given tool to evaluate an expression.
func (engine *K8sEngine) runExpressionTool(tool *Tool) (err error) {
	engine.infof("begin run ExpressionTool: %v", tool.Task.Root.ID)
	err = tool.evaluateExpression()
	if err != nil {
		return engine.errorf("failed to evaluate expression for tool: %v; error: %v", tool.Task.Root.ID, err)
	}
	err = engine.dispatchTaskJob(tool)
	if err != nil {
		return engine.errorf("failed to dispatch task job: %v; error: %v", tool.Task.Root.ID, err)
	}
	engine.infof("end run ExpressionTool: %v", tool.Task.Root.ID)
	return nil
}
