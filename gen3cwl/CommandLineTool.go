package gen3cwl

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// Tool defines an interface for workflow tools
type Tool interface {
	GenerateCommand() error
	GatherOutputs() error
}

// CommandLineTool represents class described as "CommandLineTool".
type CommandLineTool struct {
	Outdir     string // Given by context
	Root       *cwl.Root
	Parameters cwl.Parameters
	Command    *exec.Cmd
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
	var kubeconfig *string
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
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
func (tool *CommandLineTool) makeJobName() string {
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
func (tool *CommandLineTool) getSidecarArgs() []string {
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
// in particular wait until run.sh exists (run.sh is the command for the CommandLineTool)
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

// RunK8sJob runs the command line tool in a container as a k8s job with a sidecar container to generate command, install s3fs/goofys and mount bucket
func (tool *CommandLineTool) RunK8sJob() error {
	jobName := tool.makeJobName() // slightly modified Root.ID
	jobsClient := getJobClient()

	fmt.Println("\tCreating k8s job spec..")
	// Simple, minimal config
	batchJob := &batchv1.Job{
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
							Args:            tool.getSidecarArgs(),
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
	fmt.Println("\tCreating job..")
	newJob, err := jobsClient.Create(batchJob)
	if err != nil {
		return err
	}
	fmt.Println("\tSuccessfully created job.")
	fmt.Printf("\tNew job name: %v\n", newJob.Name)
	fmt.Printf("\tNew job UID: %v\n", newJob.GetUID())
	fmt.Printf("\tNew job status: %v\n", jobStatusToString(&newJob.Status))
	return nil
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
func (tool *CommandLineTool) GenerateCommand() error {

	// FIXME: this procedure ONLY adjusts to "baseCommand" job
	fmt.Printf("\n\tEnsuring arguments..")
	arguments := tool.ensureArguments()
	// fmt.Print("arguments")
	// fmt.Print(tool.Parameters)
	fmt.Printf("\n\tEnsuring inputs..")
	priors, inputs, err := tool.ensureInputs()
	if err != nil {
		return fmt.Errorf("failed to ensure required inputs: %v", err)
	}
	// fmt.Print("Inputs")
	// fmt.Print(inputs)
	fmt.Printf("\n\tGenerating basic command..")
	cmd, err := tool.generateBasicCommand(priors, arguments, inputs)
	tool.Command = cmd
	fmt.Printf("\n\tCommand: %v %v\n", cmd.Path, cmd.Args)
	if err != nil {
		return fmt.Errorf("failed to generate command struct: %v", err)
	}
	return nil
}

// ensureArguments ...
func (tool *CommandLineTool) ensureArguments() []string {
	result := []string{}
	sort.Sort(tool.Root.Arguments)
	for i, arg := range tool.Root.Arguments {
		if arg.Binding != nil && arg.Binding.ValueFrom != nil {
			tool.Root.Arguments[i].Value = tool.AliasFor(arg.Binding.ValueFrom.Key())
		}
		result = append(result, tool.Root.Arguments[i].Flatten()...)
	}
	return result
}

// ensureInputs ...
func (tool *CommandLineTool) ensureInputs() (priors []string, result []string, err error) {
	sort.Sort(tool.Root.Inputs)
	for _, in := range tool.Root.Inputs {
		in, err = tool.ensureInput(in)
		if err != nil {
			return priors, result, err
		}
		if in.Binding == nil {
			continue
		}
		if in.Binding.Position < 0 {
			priors = append(priors, in.Flatten()...)
		} else {
			result = append(result, in.Flatten()...)
		}
	}
	return priors, result, nil
}

// ensureInput ...
func (tool *CommandLineTool) ensureInput(input *cwl.Input) (*cwl.Input, error) {
	if provided, ok := tool.Parameters[input.ID]; ok {
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	}
	if input.Default == nil && input.Binding == nil && input.Provided == nil {
		return input, fmt.Errorf("input `%s` doesn't have default field but not provided", input.ID)
	}
	if key, needed := input.Types[0].NeedRequirement(); needed {
		for _, req := range tool.Root.Requirements {
			for _, requiredtype := range req.Types {
				if requiredtype.Name == key {
					input.RequiredType = &requiredtype
					input.Requirements = tool.Root.Requirements
				}
			}
		}
	}
	return input, nil
}

// AliasFor ...
func (tool *CommandLineTool) AliasFor(key string) string {
	switch key {
	case "GenerateCommandtime.cores":
		return "2"
	}
	return ""
}

// generateBasicCommand ...
func (tool *CommandLineTool) generateBasicCommand(priors, arguments, inputs []string) (*exec.Cmd, error) {
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

// PrintJSON pretty prints a struct as json
func PrintJSON(i interface{}) {
	see, _ := json.MarshalIndent(i, "", "   ")
	fmt.Println(string(see))
}

// func ResolveReference()

// ConstructOutputs builds the output parameters after the command has been executed
func (tool *CommandLineTool) ConstructOutputs() cwl.Parameters {
	fmt.Println("tool.Root.Outputs:")
	PrintJSON(tool.Root.Outputs)
	fmt.Println("tool.Parameters:")
	PrintJSON(tool.Parameters)
	fmt.Println("tool.Root:")
	PrintJSON(tool.Root)
	return nil
}

// GatherOutputs gather outputs from the finished task
func (tool *CommandLineTool) GatherOutputs() error {

	// If "cwl.output.json" exists on executed command directory,
	// dump the file contents on stdout.
	// This is described on official document.
	// See also https://www.commonwl.org/v1.0/CommandLineTool.html#Output_binding
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
	// See also https://www.commonwl.org/v1.0/CommandLineTool.html#File
	if err := tool.Root.Outputs.Dump(vm, tool.Command.Dir, tool.Root.Stdout, tool.Root.Stderr, os.Stdout); err != nil {
		return err
	}

	return nil
}
