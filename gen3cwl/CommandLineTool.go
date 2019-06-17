package gen3cwl

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/clientcmd"
	// k8sResource "k8s.io/apimachinery/pkg/api/resource"
)

// JobInfo - k8s job information
type JobInfo struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// utility.. for testing
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getJobClient() batchtypev1.JobInterface {
	/*
		/////// Commented out for testing locally (out-of-cluster) ///////
		// creates the in-cluster config
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	*/

	/////////// begin section for getting out-of-cluster config for testing locally ////////////
	/*
		var kubeconfig *string
		fmt.Printf("number of flags defined: %v", flag.NFlag())
		if flag.NFlag() == 0 {
			if home := homeDir(); home != "" {
				kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
			} else {
				kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
			}
			flag.Parse()
		}
	*/

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", "/Users/mattgarvin/.kube/config") // for testing locally...
	if err != nil {
		panic(err.Error())
	}
	//////////// end section for getting out-of-cluster config ////////////////////////////////

	clientset, err := kubernetes.NewForConfig(config)
	batchClient := clientset.BatchV1()
	jobsClient := batchClient.Jobs("default")
	return jobsClient
}

// replace disallowed job name characters
func (proc *Process) makeJobName() string {
	taskID := proc.Task.Root.ID
	jobName := strings.ReplaceAll(taskID, "#", "")
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ToLower(jobName)
	if proc.Task.ScatterIndex != 0 {
		// indicates this task is a scattered subtask of a task which was scattered
		// in order to not dupliate k8s job names - append suffix with ScatterIndex to job name
		jobName = fmt.Sprintf("%v-scattered-%v", jobName, proc.Task.ScatterIndex)
	}
	return jobName
}

func getBoolPointer(val bool) (pval *bool) {
	return &val
}

func getPropagationMode(val k8sv1.MountPropagationMode) (pval *k8sv1.MountPropagationMode) {
	return &val
}

// the sidecar needs to
// 1. install s3fs (goofys???) TODO // (apt-get update; apt-get install s3fs -y)
// 2. mount the s3 bucket  TODO
// 3. assemble command and save as /data/run.sh (done)
func (tool *Tool) getSidecarArgs() []string {
	toolCmd := strings.Join(tool.Command.Args, " ")
	fmt.Printf("command: %q", toolCmd)
	// to run the actual command: remove the second "echo" from the second line
	// need to add commands here to install goofys and mount the s3 bucket
	sidecarCmd := fmt.Sprintf(`
	echo sidecar is running..
	echo "echo %v" > /data/run.sh
	echo successfully created /data/run.sh
	`, "run this bash script to execute the commandlinetool")
	args := []string{
		"-c",
		sidecarCmd,
	}
	return args
}

// wait for sidecar to setup
// in particular wait until run.sh exists (run.sh is the command for the Tool)
// as soon as run.sh exists, run this script
func (proc *Process) getCLToolArgs() []string {
	args := []string{
		"-c",
		fmt.Sprintf(`
    while [[ ! -f /data/run.sh ]]; do
      echo "Waiting for sidecar to finish setting up..";
      sleep 5
    done
		echo "Sidecar setup complete! Running /data/run.sh now.."
		%v /data/run.sh
		`, proc.getCLTBash()),
	}
	return args
}

// handles the DockerRequirement if specified and returns the image to be used for the CommandLineTool
// if no image specified, returns `ubuntu` as a default image - need to ask/check if there is a better default image to use
// NOTE: presently only supporting use of the `dockerPull` CWL field
func (proc *Process) getDockerImage() string {
	for _, requirement := range proc.Task.Root.Requirements {
		if requirement.Class == "DockerRequirement" {
			if requirement.DockerPull != "" {
				// Shenglai made comment about adding `sha256` tag in order to pull exactly the latest image you want
				// ask for detail/example and ask others to see if I should implement that
				return string(requirement.DockerPull)
			}
		}
	}
	return "ubuntu"
}

// get path to bash.. it is problematic to have to deal with this
// only doing this right now temporarily so that test workflow runs
// here TODO: come up with a better solution for this
func (proc *Process) getCLTBash() string {
	if proc.getDockerImage() == "alpine" {
		return "/bin/sh"
	}
	return "/bin/bash"
}

// only set limits when they are specified in the CWL
// the "default" limits are no limits
// see: https://godoc.org/k8s.io/api/core/v1#Container
// the `Resources` field
// for k8s resource info see: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
func (proc *Process) getResourceReqs() k8sv1.ResourceRequirements {
	var cpuReq, cpuLim int64
	var memReq, memLim int64
	requests, limits := make(k8sv1.ResourceList), make(k8sv1.ResourceList)
	for _, requirement := range proc.Task.Root.Requirements {
		if requirement.Class == "ResourceRequirement" {
			// for info on quantities, see: https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity
			if requirement.CoresMin > 0 {
				cpuReq = int64(requirement.CoresMin)
				requests[k8sv1.ResourceCPU] = *k8sResource.NewQuantity(cpuReq, k8sResource.DecimalSI)
			}

			if requirement.CoresMax > 0 {
				cpuLim = int64(requirement.CoresMax)
				limits[k8sv1.ResourceCPU] = *k8sResource.NewQuantity(cpuLim, k8sResource.DecimalSI)
			}

			// Memory is provided in mebibytes (1 mebibyte is 2**20 bytes)
			// here we convert mebibytes to bytes
			if requirement.RAMMin > 0 {
				memReq = int64(requirement.RAMMin * int(math.Pow(2, 20)))
				requests[k8sv1.ResourceMemory] = *k8sResource.NewQuantity(memReq, k8sResource.DecimalSI)
			}

			if requirement.RAMMax > 0 {
				memLim = int64(requirement.RAMMax * int(math.Pow(2, 20)))
				limits[k8sv1.ResourceMemory] = *k8sResource.NewQuantity(memLim, k8sResource.DecimalSI)
			}
		}
	}

	// sanity check for negative requirements
	reqVals := []int64{cpuReq, cpuLim, memReq, memLim}
	for _, val := range reqVals {
		if val < 0 {
			panic("negative memory or cores requirement specified")
		}
	}

	// verify valid bounds if both min and max specified
	if memLim > 0 && memReq > 0 && memLim < memReq {
		panic("memory maximum specified less than memory minimum specified")
	}

	if cpuLim > 0 && cpuReq > 0 && cpuLim < cpuReq {
		panic("cores maximum specified less than cores minimum specified")
	}

	resourceReqs := k8sv1.ResourceRequirements{}
	// only want to populate values if specified in the CWL
	if len(requests) > 0 {
		resourceReqs.Requests = requests
	}
	if len(limits) > 0 {
		resourceReqs.Limits = limits
	}
	/*
		resourceReqs := k8sv1.ResourceRequirements{
			Requests: k8sv1.ResourceList{
				k8sv1.ResourceCPU:    *k8sResource.NewQuantity(cpuReq, k8sResource.DecimalSI),
				k8sv1.ResourceMemory: *k8sResource.NewQuantity(memReq, k8sResource.DecimalSI),
			},
			Limits: k8sv1.ResourceList{
				k8sv1.ResourceCPU:    *k8sResource.NewQuantity(cpuLim, k8sResource.DecimalSI),
				k8sv1.ResourceMemory: *k8sResource.NewQuantity(memLim, k8sResource.DecimalSI),
			},
		}
	*/
	return resourceReqs
}

func createJobSpec(proc *Process) (batchJob *batchv1.Job, err error) {
	jobName := proc.makeJobName() // slightly modified Root.ID
	proc.JobName = jobName
	fmt.Printf("Pulling image %v for task %v\n", proc.getDockerImage(), proc.Task.Root.ID)
	batchJob = &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   jobName,
			Labels: make(map[string]string), // to be populated - labels for job object
		},
		Spec: batchv1.JobSpec{
			Template: k8sv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   jobName,
					Labels: make(map[string]string), // to be populated - labels for pod object
				},
				Spec: k8sv1.PodSpec{
					RestartPolicy: k8sv1.RestartPolicyNever,
					Volumes: []k8sv1.Volume{
						{
							Name: "shared-data", // implicitly set to be an emptyDir
						},
					},
					Containers: []k8sv1.Container{
						{
							Name:            "commandlinetool",
							Image:           proc.getDockerImage(),
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),
							Command: []string{
								proc.getCLTBash(), // get path to bash for docker image (needs better solution)
							},
							Args:      proc.getCLToolArgs(), // need function here to identify path to bash based on docker image
							Resources: proc.getResourceReqs(),
							VolumeMounts: []k8sv1.VolumeMount{
								{
									Name:             "shared-data",
									MountPath:        "/data",
									MountPropagation: getPropagationMode(k8sv1.MountPropagationHostToContainer),
								},
							},
						},
						{
							Name:  "sidecar",
							Image: "ubuntu",
							Command: []string{
								"/bin/bash",
							},
							Args:            proc.Tool.getSidecarArgs(),
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullIfNotPresent),
							SecurityContext: &k8sv1.SecurityContext{
								Privileged: getBoolPointer(true),
							},
							VolumeMounts: []k8sv1.VolumeMount{
								{
									Name:             "shared-data",
									MountPath:        "/data",
									MountPropagation: getPropagationMode(k8sv1.MountPropagationBidirectional),
								},
							},
						},
					},
				},
			},
		},
	}
	return batchJob, nil
}

// RunK8sJob runs the CommandLineTool in a container as a k8s job with a sidecar container to write command to run.sh, install s3fs/goofys and mount bucket
func (engine K8sEngine) RunK8sJob(proc *Process) error {
	fmt.Println("\tCreating k8s job spec..")
	batchJob, nil := createJobSpec(proc)

	jobsClient := getJobClient() // does this need to happen for each job? or just once, so every job uses the same jobsClient?

	fmt.Println("\tRunning k8s job..")
	newJob, err := jobsClient.Create(batchJob)
	if err != nil {
		fmt.Printf("\tError creating job: %v\n", err)
		return err
	}
	fmt.Println("\tSuccessfully created job.")
	fmt.Printf("\tNew job name: %v\n", newJob.Name)
	fmt.Printf("\tNew job UID: %v\n", newJob.GetUID())
	proc.JobID = string(newJob.GetUID())
	proc.JobName = newJob.Name
	// fmt.Printf("\tNew job status: %v\n", jobStatusToString(&newJob.Status))
	return nil
}

func getJobByID(jc batchtypev1.JobInterface, jobid string) (*batchv1.Job, error) {
	jobs, err := jc.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, job := range jobs.Items {
		if jobid == string(job.GetUID()) {
			return &job, nil
		}
	}
	return nil, fmt.Errorf("job with jobid %s not found", jobid)
}

// ListenForDone listens to k8s until the job status is "Completed"
// when complete, calls a function to collect output and update engine's proc stacks
func (engine *K8sEngine) ListenForDone(proc *Process) (err error) {
	fmt.Println("\tListening for job to finish..")
	status := ""
	for status != "Completed" {
		jobInfo, err := getJobStatusByID(proc.JobID)
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

func getJobStatusByID(jobid string) (*JobInfo, error) {
	job, err := getJobByID(getJobClient(), jobid)
	if err != nil {
		return nil, err
	}
	ji := JobInfo{}
	ji.Name = job.Name
	ji.UID = string(job.GetUID())
	ji.Status = jobStatusToString(&job.Status)
	return &ji, nil
}

func jobStatusToString(status *batchv1.JobStatus) string {
	if status == nil {
		return "Unknown"
	}

	// https://kubernetes.io/docs/api-reference/batch/v1/definitions/#_v1_jobstatus
	if status.Succeeded >= 1 {
		return "Completed"
	}
	if status.Failed >= 1 {
		return "Failed"
	}
	if status.Active >= 1 {
		return "Running"
	}
	return "Unknown"
}

// NotGenerateCommand is the original function written to generate commands for CLT's
// it is basically busted so am writing a new function to generate commands correctly in *all* cases
func (tool *Tool) NotGenerateCommand() error {

	// FIXME: this procedure ONLY adjusts to "baseCommand" job
	// handles arguments
	fmt.Println("ensuring arguments..")
	arguments := tool.ensureArguments()

	// handles inputs
	fmt.Println("ensuring inputs..")
	priors, inputs, err := tool.ensureInputs()
	if err != nil {
		return fmt.Errorf("failed to ensure required inputs: %v", err)
	}

	fmt.Println("generating basic command..")
	cmd, err := tool.generateBasicCommand(priors, arguments, inputs)
	tool.Command = cmd
	fmt.Printf("\n\tCommand: %v\n", cmd.Args)
	if err != nil {
		return fmt.Errorf("failed to generate command struct: %v", err)
	}
	return nil
}

/*
Notes on generating commands for CLTs
- baseCommand contains leading arguments
- inputs and arguments mix together and are ordered via position specified in each binding
- the rules for sorting when no position is specified are truly ambiguous, so
---- presently only supporting sorting inputs/arguments via position and no other key
---- later can implement sorting based on additional keys, but not in first iteration here

Sketch of Steps:
0. cmdElts := make([]CommandElement, 0)
1. Per argument, construct CommandElement -> cmdElts = append(cmdElts, cmdElt)
2. Per input, construct CommandElement -> cmdElts = append(cmdElts, cmdElt)
3. Sort(cmdElts) -> using Position field values (or whatever sorting key we want to use later on)
4. Iterate through sorted cmdElts -> cmd = append(cmd, cmdElt.Value...)
5. cmd = append(baseCmd, cmd...)
6. return cmd
*/

// CommandElement represents an input/argument on the commandline for commandlinetools
type CommandElement struct {
	Position    int      // position from binding
	ArgPosition int      // index from arguments list, if argument
	Value       []string // representation of this input/arg on the commandline (after any/all valueFrom, eval, prefix, separators, shellQuote, etc. has been resolved)
}

func (tool *Tool) getCmdElts() (cmdElts CommandElements, err error) {
	// 0
	cmdElts = make([]*CommandElement, 0)

	// 1. handle arguments
	argElts, err := tool.getArgElts() // good - need to test
	if err != nil {
		return nil, err
	}
	cmdElts = append(cmdElts, argElts...)

	// 2. handle inputs
	inputElts, err := tool.getInputElts() // TODO -
	if err != nil {
		return nil, err
	}
	cmdElts = append(cmdElts, inputElts...)

	return cmdElts, nil
}

// HERE TODO - Monday - construct, collect, return CommandElement per input
// NOTE: how to handle optional inputs?
func (tool *Tool) getInputElts() (cmdElts CommandElements, err error) {
	cmdElts = make([]*CommandElement, 0)
	for _, input := range tool.Root.Inputs {
		// no binding -> input doesn't get processed for representation on the commandline (though this input may be referenced by an argument)
		if input.Binding != nil {
			pos := input.Binding.Position         // default position is 0, as per CWL spec
			val, err := tool.getInputValue(input) // TODO - return []string which is the resolved binding (representation on commandline) for this input
			if err != nil {
				return nil, err
			}
			cmdElt := &CommandElement{
				Position: pos,
				Value:    val,
			}
			cmdElts = append(cmdElts, cmdElt)
		}
	}
	return cmdElts, nil
}

// TODO - incomplete - see array and object cases
func (tool *Tool) getInputValue(input *cwl.Input) (val []string, err error) {
	// binding is non-nil
	// input sources:
	// 1. if valueFrom specified in binding, then input value taken from there
	// 2. else input value taken from input object
	// regardless of input source, the input value to work with for the binding is stored in input.Provided.Raw
	// need a type switch to cover all the possible cases
	// recall a few different binding rules apply for different input types
	// see: https://www.commonwl.org/v1.0/CommandLineTool.html#CommandLineBinding

	fmt.Println("here is an input:")
	PrintJSON(input)
	fmt.Println("here is input.Provided:")
	PrintJSON(input.Provided)

	/*
		Steps:
		1. identify type
		2. retrieve value based on type -> convert to string based on type -> collect in val
		3. if prefix specified -> handle prefix based on type (also handle `separate` if specified) -> collect in val
		4. if array and separator specified - handle separator -> collect in val
		5. handle shellQuote -> not handling in first iteration
	*/

	var s string
	rawInput := input.Provided.Raw
	switch input.Types[0].Type {
	case "array": // TODO - Presently bindings on array inputs not supported
		/*
			if input.Types[0].Binding != nil {
				// apply binding to each element of the array individually
				binding := input.Types[0].Binding
			} else {
				// apply binding to array as a whole - prefix, separate or not, separator
				// need to extract elements from list and convert to string
				if input.Binding.Separator != "NOT SPECIFIED" {

				}
				if input.Binding.Prefix != "" {

				}
			}
		*/
		return nil, fmt.Errorf("bindings for array inputs presently not supported")
	case "object": // TODO - presently bindings on object inputs not supported
		// "Add prefix only, and recursively add object fields for which inputBinding is specified."
		// presently not supported
		return nil, fmt.Errorf("inputs of type 'object' not supported. input: %v", rawInput)
	case "null": // okay
		// "Add nothing."
		return val, nil
	case "boolean": // okay
		if input.Binding.Prefix == "" {
			return nil, fmt.Errorf("boolean input provided but no prefix provided")
		}
		boolVal, err := getBoolFromRaw(rawInput)
		if err != nil {
			return nil, err
		}
		// "if true, add 'prefix' to the commandline. If false, add nothing."
		if boolVal {
			val = append(val, input.Binding.Prefix)
		}
		return val, nil
	case "string", "number": // okay
		s, err = getValFromRaw(rawInput)
	case "File", "Directory": // okay
		s, err = getPathFromRaw(rawInput)
	}
	// string/number and file/directory share the same processing here
	// other cases end with return statements
	if err != nil {
		return nil, err
	}
	if input.Binding.Prefix != "" {
		val = append(val, input.Binding.Prefix)
	}
	val = append(val, s)
	if !input.Binding.Separate {
		val = []string{strings.Join(val, "")}
	}
	return val, nil
}

// called in getInputValue()
func getBoolFromRaw(rawInput interface{}) (boolVal bool, err error) {
	boolVal, ok := rawInput.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected data type for input specified as bool: %v; %T", rawInput, rawInput)
	}
	return boolVal, nil
}

// called in getInputValue()
func getPathFromRaw(rawInput interface{}) (path string, err error) {
	switch rawInput.(type) {
	case string:
		path = rawInput.(string)
	case *File:
		fileObj := rawInput.(*File)
		path = fileObj.Path
	default:
		path, err = GetPath(rawInput)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve file or directory path from object of type %T with value %v", rawInput, rawInput)
		}
	}
	return path, nil
}

// called in getInputValue()
func getValFromRaw(rawInput interface{}) (val string, err error) {
	switch rawInput.(type) {
	case string:
		val = rawInput.(string)
	case int:
		val = strconv.Itoa(rawInput.(int))
	case float64:
		val = strconv.FormatFloat(rawInput.(float64), 'f', -1, 64)
	default:
		return "", fmt.Errorf("unexpected data type for input specified as number or string: %v; %T", rawInput, rawInput)
	}
	return val, nil
}

// collect CommandElement objects from arguments
func (tool *Tool) getArgElts() (cmdElts CommandElements, err error) {
	cmdElts = make([]*CommandElement, 0) // this might be redundant - basic q: do I need to instantiate this array if it's a named output?
	for i, arg := range tool.Root.Arguments {
		pos := 0 // if no position specified the default is zero, as per CWL spec
		if arg.Binding != nil {
			pos = arg.Binding.Position
		}
		val, err := tool.getArgValue(arg) // okay
		if err != nil {
			return nil, err
		}
		cmdElt := &CommandElement{
			Position:    pos,
			ArgPosition: i + 1, // beginning at 1 so that can detect nil/zero value of 0
			Value:       val,
		}
		cmdElts = append(cmdElts, cmdElt)
	}
	return cmdElts, nil
}

// gets value from an argument - i.e., returns []string containing strings which will be put on the commandline to represent this argument
func (tool *Tool) getArgValue(arg cwl.Argument) (val []string, err error) {
	// cases:
	// either a string literal or an expression (okay)
	// OR a binding with valueFrom field specified (okay)
	val = make([]string, 0)
	if arg.Value != "" {
		// implies string literal or expression to eval - okay - see NOTE at typeSwitch

		// NOTE: *might* need to check "$(" or "${" instead of just "$"
		if strings.HasPrefix(arg.Value, "$") {
			// expression to eval - here `self` is null - no additional context to load - just need to eval in inputsVM
			result, err := EvalExpression(arg.Value, tool.Root.InputsVM)
			if err != nil {
				return nil, err
			}
			// NOTE: what type can I expect the result to be here? - hopefully string or []string - need to test and find additional examples to work with
			switch result.(type) {
			case string:
				val = append(val, result.(string))
			case []string:
				val = append(val, result.([]string)...)
			default:
				return nil, fmt.Errorf("unexpected type returned by argument expression: %v; %v; %T", arg.Value, result, result)
			}
		} else {
			// string literal - no processing to be done
			val = append(val, arg.Value)
		}
	} else {
		// get value from `valueFrom` field which may itself be a string literal, an expression, or a string which contains one or more expressions
		resolvedText, err := tool.resolveExpressions(arg.Binding.ValueFrom.String)
		if err != nil {
			return nil, err
		}

		// handle shellQuote - default value is true
		if arg.Binding.ShellQuote {
			resolvedText = "\"" + resolvedText + "\""
		}

		// capture result
		val = append(val, resolvedText)
	}
	return val, nil
}

// resolveExpressions processes a text field which may or may not be
// - one expression
// - a string literal
// - a string which contains one or more separate JS expressions, each wrapped like $(...)
// presently writing simple case to return a string only for use in the argument valueFrom case
// can easily extend in the future to be used for any field, to return any kind of value
// NOTE: should work - needs to be tested more
// algorithm works in goplayground: https://play.golang.org/p/YOv-K-qdL18
func (tool *Tool) resolveExpressions(inText string) (outText string, err error) {
	r := bufio.NewReader(strings.NewReader(inText))
	var c0, c1, c2 string
	var done bool
	image := make([]string, 0)
	for !done {
		nextRune, _, err := r.ReadRune()
		if err != nil {
			if err == io.EOF {
				done = true
			} else {
				return "", err
			}
		}
		c0, c1, c2 = c1, c2, string(nextRune)
		if c1 == "$" && c2 == "(" && c0 != "\\" {
			// indicates beginning of expression block

			// read through to the end of this expression block
			expression, err := r.ReadString(')')
			if err != nil {
				return "", err
			}

			// get full $(...) expression
			expression = c1 + c2 + expression

			// eval that thing
			result, err := EvalExpression(expression, tool.Root.InputsVM)
			if err != nil {
				return "", err
			}

			// result ought to be a string
			val, ok := result.(string)
			if !ok {
				return "", fmt.Errorf("js embedded in string did not return a string")
			}

			// cut off trailing "$" that had already been collected
			image = image[:len(image)-1]

			// collect resulting string
			image = append(image, val)
		} else {
			if !done {
				// checking done so as to not collect null value
				image = append(image, string(c2))
			}
		}
	}
	// get resolved string value
	outText = strings.Join(image, "")
	return outText, nil
}

// GenerateCommand ..
func (tool *Tool) GenerateCommand() (err error) {
	cmdElts, err := tool.getCmdElts() // 1. get arguments (okay) 2. get inputs from bindings - TODO
	if err != nil {
		return err
	}
	fmt.Println("here are cmdElts:")
	PrintJSON(cmdElts)

	// 3. Sort the command elements by position (okay)
	sort.Sort(cmdElts)

	/*
		4. Iterate through sorted cmdElts -> cmd = append(cmd, cmdElt.Args...) (okay)
		5. cmd = append(baseCmd, cmd...) (okay)
		6. return cmd (okay)
	*/
	cmd := tool.Root.BaseCommands // BaseCommands is []string - zero length if no BaseCommand specified
	for _, cmdElt := range cmdElts {
		cmd = append(cmd, cmdElt.Value...)
	}
	tool.Command = exec.Command(cmd[0], cmd[1:]...)
	return nil
}

// define this type and methods for sort.Interface so these CommandElements can be sorted by position
type CommandElements []*CommandElement

// from first example at: https://golang.org/pkg/sort/
func (cmdElts CommandElements) Len() int           { return len(cmdElts) }
func (cmdElts CommandElements) Swap(i, j int)      { cmdElts[i], cmdElts[j] = cmdElts[j], cmdElts[i] }
func (cmdElts CommandElements) Less(i, j int) bool { return cmdElts[i].Position < cmdElts[j].Position }

// ensureArguments ...
// NOTE: gut this
func (tool *Tool) ensureArguments() []string {
	result := []string{}
	sort.Sort(tool.Root.Arguments)
	for i, arg := range tool.Root.Arguments {
		if arg.Binding != nil && arg.Binding.ValueFrom != nil {
			tool.Root.Arguments[i].Value = tool.AliasFor(arg.Binding.ValueFrom.Key()) // unsure of this AliasFor() bit
		}
		result = append(result, tool.Root.Arguments[i].Flatten()...)
	}
	return result
}

// ensureInputs ...
// here is where bindings/inputs get "resolved" by cwl.go library
// NOTE: this is probably going to need to be totally overhauled
func (tool *Tool) ensureInputs() (priors []string, result []string, err error) {
	sort.Sort(tool.Root.Inputs)
	for _, in := range tool.Root.Inputs {
		if in.Binding == nil {
			continue
		}
		// in.Flatten() is where the input gets resolved to how it should appear on the commandline
		// need to check various cases to make sure that this actually handles different kinds of input properly
		// NOTE: there's an Input.flatten() method as well as an Input.Flatten() method - what gives?
		fmt.Printf("flattening input %v ..\n", in.ID)
		PrintJSON(in)
		fmt.Println("provided:")
		PrintJSON(in.Provided)
		if in.Binding.Position < 0 {
			priors = append(priors, in.Flatten()...)
		} else {
			result = append(result, in.Flatten()...)
		}
	}
	return priors, result, nil
}

// AliasFor ... ??? - seems incomplete
// this looks like a bad idea
func (tool *Tool) AliasFor(key string) string {
	switch key {
	case "GenerateCommandtime.cores":
		return "2"
	}
	return ""
}

// generateBasicCommand ...
// this needs to be examined as well
func (tool *Tool) generateBasicCommand(priors, arguments, inputs []string) (*exec.Cmd, error) {
	if len(tool.Root.BaseCommands) == 0 {
		return exec.Command("bash", "-c", tool.Root.Arguments[0].Binding.ValueFrom.Key()), nil
	}

	// Join all slices
	oneline := []string{}
	oneline = append(oneline, tool.Root.BaseCommands...)
	oneline = append(oneline, priors...)
	oneline = append(oneline, arguments...)
	oneline = append(oneline, inputs...)

	return exec.Command(oneline[0], oneline[1:]...), nil
}

// GatherOutputs gather outputs from the finished task
// NOTE: currently not being used
func (tool *Tool) GatherOutputs() error {

	// If "cwl.output.json" exists on executed command directory,
	// dump the file contents on stdout.
	// This is described on official document.
	// See also https://www.commonwl.org/v1.0/Tool.html#Output_binding
	whatthefuck := filepath.Join(tool.Command.Dir, "cwl.output.json")
	if defaultout, err := os.Open(whatthefuck); err == nil {
		defer defaultout.Close()
		if _, err := io.Copy(os.Stdout, defaultout); err != nil {
			return err
		}
		return nil
	}

	// Load Contents as JavaScript RunTime if needed.
	vm, err := tool.Root.Outputs.LoadContents(tool.Command.Dir)
	if err != nil {
		return err
	}

	// CWL wants to dump metadata of outputs with type="File"
	// See also https://www.commonwl.org/v1.0/Tool.html#File
	if err := tool.Root.Outputs.Dump(vm, tool.Command.Dir, tool.Root.Stdout, tool.Root.Stderr, os.Stdout); err != nil {
		return err
	}

	return nil
}
