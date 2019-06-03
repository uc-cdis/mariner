package gen3cwl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/clientcmd"
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
func (tool *Tool) makeJobName() string {
	taskID := tool.Root.ID
	jobName := strings.ReplaceAll(taskID, "#", "")
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ToLower(jobName)
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
	// to run the actual command: remove the second "echo" from the second line
	// need to add commands here to install goofys and mount the s3 bucket
	sidecarCmd := fmt.Sprintf(`
	echo sidecar is running..
	echo "echo %v" > /data/run.sh
	echo successfully created /data/run.sh
	`, toolCmd)
	args := []string{
		"-c",
		sidecarCmd,
	}
	return args
}

// wait for sidecar to setup
// in particular wait until run.sh exists (run.sh is the command for the Tool)
// as soon as run.sh exists, run this script
func getCLToolArgs() []string {
	args := []string{
		"-c",
		`
    while [[ ! -f /data/run.sh ]]; do
      echo "Waiting for sidecar to finish setting up..";
      sleep 5
    done
		echo "Sidecar setup complete! Running /data/run.sh now.."
		/bin/bash /data/run.sh
		`,
	}
	return args
}

func createJobSpec(proc *Process) (batchJob *batchv1.Job, err error) {
	jobName := proc.Tool.makeJobName() // slightly modified Root.ID
	proc.JobName = jobName
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
							Name:  "commandlinetool",
							Image: "ubuntu", // need to handle cwl docker-requirements and pick a good default image
							Command: []string{
								"/bin/bash",
							},
							Args:            getCLToolArgs(),
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),
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

	// testing this function now (10:37am)
	proc.CollectOutput()

	fmt.Println("\tFinished collecting output.")
	// PrintJSON(proc.Task.Outputs)

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

// GenerateCommand ...
func (tool *Tool) GenerateCommand() error {

	// FIXME: this procedure ONLY adjusts to "baseCommand" job
	arguments := tool.ensureArguments()
	priors, inputs, err := tool.ensureInputs()
	if err != nil {
		return fmt.Errorf("failed to ensure required inputs: %v", err)
	}
	cmd, err := tool.generateBasicCommand(priors, arguments, inputs)
	tool.Command = cmd
	fmt.Printf("\n\tCommand: %v\n", cmd.Args)
	if err != nil {
		return fmt.Errorf("failed to generate command struct: %v", err)
	}
	return nil
}

// ensureArguments ...
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
// here is where bindings/inputs get resolved
func (tool *Tool) ensureInputs() (priors []string, result []string, err error) {
	sort.Sort(tool.Root.Inputs)
	for _, in := range tool.Root.Inputs {
		if in.Binding == nil {
			continue
		}
		// in.Flatten() is where the input gets resolved to how it should appear on the commandline
		// need to check various cases to make sure that this actually handles different kinds of input properly
		// NOTE: there's an Input.flatten() method as well as an Input.Flatten() method - what gives?
		if in.Binding.Position < 0 {
			priors = append(priors, in.Flatten()...)
		} else {
			result = append(result, in.Flatten()...)
		}
	}
	return priors, result, nil
}

// AliasFor ... ??? - seems incomplete
func (tool *Tool) AliasFor(key string) string {
	switch key {
	case "GenerateCommandtime.cores":
		return "2"
	}
	return ""
}

// generateBasicCommand ...
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
