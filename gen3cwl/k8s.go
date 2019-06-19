package gen3cwl

import (
	"fmt"
	"math"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// this file contains code for interacting with k8s cluster via k8s api
// e.g., get cluster config, handle runtime requirements, create job spec, execute job, get job info/status

// JobInfo - k8s job information
type JobInfo struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Status string `json:"status"`
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

// handle EnvVarRequirement if specified - need to test
func (proc *Process) getEnv() (env []k8sv1.EnvVar) {
	for _, requirement := range proc.Tool.Root.Requirements {
		if requirement.Class == "EnvVarRequirement" {
			for _, envDef := range requirement.EnvDef {
				varValue, err := proc.Tool.resolveExpressions(envDef.Value) // resolves any expression(s) - if no expressions, returns original text
				if err != nil {
					panic("failed to resolve expressions in envVar def")
				}
				envVar := k8sv1.EnvVar{
					Name:  envDef.Name,
					Value: varValue,
				}
				env = append(env, envVar)
			}
		}
	}
	return env
}

// probably should do some more careful error handling with the various get*() functions to populate job spec here
// those functions should return the value and an error - or maybe panic works just fine
// something to think about
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
							Env:       proc.getEnv(),        // set any environment variables if specified
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

// utility.. for testing
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
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
//
// NOTE: presently only supporting req's for cpu cores and RAM - need to implement outdir and tmpdir and whatever other fields are allowed
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
	return resourceReqs
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

func GetJobStatusByID(jobid string) (*JobInfo, error) {
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
