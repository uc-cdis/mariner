package mariner

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
)

// this file contains code for interacting with k8s cluster via k8s api
// e.g., get cluster config, handle runtime requirements, create job spec, execute job, get job info/status
// NOTE: clean up the code - move all the config/spec-related things into a separate file

// JobInfo - k8s job information
type JobInfo struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// probably can come up with a better ID for a workflow, but for now this will work
// can't really generate a workflow ID from the given packed workflow since the top level workflow is always called "#main"
// so not exactly sure how to label the workflow runs besides a timestamp
func getS3Prefix(content WorkflowRequest) (prefix string) {
	now := time.Now()
	timeStamp := fmt.Sprintf("%v-%v-%v_%v-%v-%v", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	prefix = fmt.Sprintf("/%v/%v/", content.ID, timeStamp)
	return prefix
}

// run bash setup script in sidecar container to mount s3 bucket to shared volume at "/data/"
func getS3SidecarArgs() []string {
	args := []string{
		fmt.Sprintf(`./s3sidecarDockerrun.sh`),
	}
	return args
}

// analogous to task main container args - maybe restructure code, less redundancy
func getEngineArgs(prefix string) []string {
	args := []string{
		"-c",
		fmt.Sprintf(`
    while [[ ! -f /data/request.json ]]; do
      echo "Waiting for mariner-engine-sidecar to finish setting up..";
      sleep 1
    done
		echo "Sidecar setup complete! Running mariner-engine now.."
		/mariner run %v
		`, prefix),
	}
	return args
}

// for mounting aws-user-creds secret to s3sidecar
// config
var awscreds = k8sv1.EnvVarSource{
	SecretKeyRef: &k8sv1.SecretKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: "workflow-bot-g3auto",
		},
		Key: "awsusercreds.json",
	},
}

// DispatchWorkflowJob runs a workflow provided in mariner api request
// TODO - move details to a config file
func DispatchWorkflowJob(content WorkflowRequest) error {
	jobsClient := getJobClient()
	S3Prefix := getS3Prefix(content) // bc of timestamp -> need to call this exactly once, and then pass that generated prefix to wherever elsewhere needed - ow timestamp changes
	request, err := json.Marshal(content)
	if err != nil {
		panic("failed to marshal request body (workflow content) into json")
	}
	batchJob := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{ // preprocess
			Kind:       "Job",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{ // https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta
			Name: "test-workflow", // replace - preprocess
			Labels: map[string]string{
				"app": "mariner-engine", // config
			}, // NOTE: what other labels should be here?
		},
		Spec: batchv1.JobSpec{
			Template: k8sv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workflow", // replace - preprocess
					Labels: map[string]string{
						"app": "mariner-engine", // config
					}, // NOTE: what other labels should be here?
				},
				Spec: k8sv1.PodSpec{
					ServiceAccountName: "mariner-service-account", // see: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
					RestartPolicy:      k8sv1.RestartPolicyNever,  // config
					Volumes: []k8sv1.Volume{
						{
							Name: "shared-data", // preprocess
							// EmptyDir: &k8sv1.EmptyDirVolumeSource{}, // can't construct struct literal with promoted field
						},
						{
							Name: "mariner-config", // preprocess
							/*
								ConfigMap: &k8sv1.ConfigMapVolumeSource{ // can't construct struct literal with promoted field
									Name: "mariner-config",
									Items: []k8sv1.KeyToPath{
										{
											Key:  "config",
											Path: "mariner.json",
										},
									},
								},
							*/
						},
					},
					Containers: []k8sv1.Container{
						{
							Name:            "mariner-engine",                       // in config
							Image:           "quay.io/cdis/mariner-engine:feat_k8s", // in config
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),     // in config
							Command: []string{ // in config
								"/bin/sh",
							},
							Env: []k8sv1.EnvVar{ // in pre-processing
								{
									Name:  "GEN3_NAMESPACE",
									Value: os.Getenv("GEN3_NAMESPACE"),
								},
							},
							Args: getEngineArgs(S3Prefix), // HERE - in pre-processing -> OR put in bash script or something
							// HERE - TODO - add sensible resource requirements here - ask devops
							VolumeMounts: []k8sv1.VolumeMount{ // in config
								{
									Name:             "shared-data",
									MountPath:        "/data",
									MountPropagation: getPropagationMode(k8sv1.MountPropagationHostToContainer),
								},
								{
									Name:      "mariner-config",
									MountPath: "/mariner.json",
									ReadOnly:  true,
								},
							},
						},
						{
							Name:  "mariner-s3sidecar",                       // in config
							Image: "quay.io/cdis/mariner-s3sidecar:feat_k8s", // in config
							Command: []string{ // in config
								"/bin/sh",
							},
							Args: getS3SidecarArgs(), // in config -> calls bash setup script
							Env: []k8sv1.EnvVar{
								{
									Name:  "S3PREFIX", // in preprocessing
									Value: S3Prefix,   // see last line of mariner-engine-sidecar dockerfile -> RUN goofys workflow-engine-garvin:$S3PREFIX /data
								},
								{
									Name:      "AWSCREDS", // in preprocessing
									ValueFrom: &awscreds,
								},
								{
									Name:  "MARINER_COMPONENT", // in preprocessing
									Value: "ENGINE",
								},
								{
									Name:  "WORKFLOW_REQUEST", // in proprocessing body of POST http request made to api
									Value: string(request),
								},
							},
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways), // in config
							SecurityContext: &k8sv1.SecurityContext{ // in config
								Privileged: getBoolPointer(true), // HERE - Q: run as user? run as group?
							},
							VolumeMounts: []k8sv1.VolumeMount{ // in config
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
	newJob, err := jobsClient.Create(batchJob)
	if err != nil {
		fmt.Printf("\tError creating job: %v\n", err)
		return err
	}
	fmt.Println("\tSuccessfully created job.")
	fmt.Printf("\tNew job name: %v\n", newJob.Name)
	fmt.Printf("\tNew job UID: %v\n", newJob.GetUID())
	return nil
}

// RunK8sJob runs the CommandLineTool in a container as a k8s job with a sidecar container to write command to run.sh, install s3fs/goofys and mount bucket
func (engine K8sEngine) DispatchTaskJob(proc *Process) error {
	fmt.Println("\tCreating k8s job spec..")
	batchJob, nil := engine.createJobSpec(proc)

	jobsClient := getJobClient()

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
	return nil
}

func getJobClient() batchtypev1.JobInterface {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	batchClient := clientset.BatchV1()
	// provide k8s namespace in which to dispatch jobs
	// namespace is inherited from whatever namespace the mariner-server was deployed in
	jobsClient := batchClient.Jobs(os.Getenv("GEN3_NAMESPACE"))
	return jobsClient
}

// wait for sidecar to setup
// in particular wait until run.sh exists (run.sh is the command for the Tool)
// as soon as run.sh exists, run this script
// HERE TODO - put this in a bash script
func (proc *Process) getCLToolArgs() []string {
	args := []string{
		"-c",
		fmt.Sprintf(`
    while [[ ! -f %vrun.sh ]]; do
      echo "Waiting for sidecar to finish setting up..";
      sleep 5
    done
		echo "Sidecar setup complete! Running command script now.."
		cd %v
		echo "running command $(cat %vrun.sh)"
		%v %vrun.sh
		echo "commandlinetool has finished running" > %vdone
		`, proc.Tool.WorkingDir, proc.Tool.WorkingDir, proc.Tool.WorkingDir, proc.getCLTBash(), proc.Tool.WorkingDir, proc.Tool.WorkingDir),
	}
	return args
}

// handle EnvVarRequirement if specified - need to test
// see: https://godoc.org/k8s.io/api/core/v1#Container
// and: https://godoc.org/k8s.io/api/core/v1#EnvVar
// and: https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/
func (proc *Process) getEnv() (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{}
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

// NOTE: need to do some more careful error handling with the various get*() functions to populate job spec here
// ----- those functions should return the value and an error - or maybe panic works just fine
// ----- something to think about
// l8est NOTE: use a CONFIG file to store all these horrific details, then just cleanly read from config doc to populate job spec
// see: https://godoc.org/k8s.io/api/core/v1#Container
func (engine *K8sEngine) createJobSpec(proc *Process) (batchJob *batchv1.Job, err error) {
	jobName := proc.makeJobName() // slightly modified Root.ID
	proc.JobName = jobName
	fmt.Printf("Pulling image %v for task %v\n", proc.getDockerImage(), proc.Task.Root.ID)
	batchJob = &batchv1.Job{
		TypeMeta: metav1.TypeMeta{ // preprocessing
			Kind:       "Job",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName, // preprocessing
			Labels: map[string]string{ // config
				"app": "mariner-task",
			}, // NOTE: what other labels should be here?
		},
		Spec: batchv1.JobSpec{
			Template: k8sv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: jobName, // preprocessing
					Labels: map[string]string{ // config
						"app": "mariner-task",
					}, // NOTE: what other labels should be here?
				},
				Spec: k8sv1.PodSpec{
					RestartPolicy: k8sv1.RestartPolicyNever,
					Volumes: []k8sv1.Volume{
						{
							Name: "shared-data", // preprocess
						},
					},
					Containers: []k8sv1.Container{
						{
							Name:            "commandlinetool",
							Image:           proc.getDockerImage(),
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),
							Command: []string{
								proc.getCLTBash(), // get path to bash for docker image (NOTE: TODO - needs better solution)
							},
							Args:      proc.getCLToolArgs(), // need function here to identify path to bash based on docker image - not sure how to navigate this
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
							Name:  "mariner-s3sidecar",
							Image: "quay.io/cdis/mariner-s3sidecar:feat_k8s", // put in config
							Command: []string{
								"/bin/sh",
							},
							Args:            getS3SidecarArgs(), // calls bash setup script - see envVars for vars referenced in the script
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),
							SecurityContext: &k8sv1.SecurityContext{
								Privileged: getBoolPointer(true),
							},
							Env: []k8sv1.EnvVar{
								{
									Name:      "AWSCREDS",
									ValueFrom: &awscreds,
								},
								{
									Name:  "S3PREFIX",
									Value: engine.S3Prefix, // mounting whole user dir to /data -> not just the dir for the task
								},
								{
									Name:  "MARINER_COMPONENT", // flag to tell setup sidecar script this is a task, not an engine job
									Value: "TASK",              // put this in config, don't hardcode it here - also potentially use different flag name
								},
								{
									Name:  "TOOL_COMMAND", // the command from the commandlinetool to actually execute
									Value: strings.Join(proc.Tool.Command.Args, " "),
								},
								{
									Name:  "TOOL_WORKING_DIR", // the tool's working directory - e.g., /data/task_id
									Value: proc.Tool.WorkingDir,
								},
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

// move to config
func getBoolPointer(val bool) (pval *bool) {
	return &val
}

// move to config
func getPropagationMode(val k8sv1.MountPropagationMode) (pval *k8sv1.MountPropagationMode) {
	return &val
}

// handles the DockerRequirement if specified and returns the image to be used for the CommandLineTool
// NOTE: if no image specified, returns `ubuntu` as a default image - need to ask/check if there is a better default image to use
// NOTE: presently only supporting use of the `dockerPull` CWL field
func (proc *Process) getDockerImage() string {
	for _, requirement := range proc.Task.Root.Requirements {
		if requirement.Class == "DockerRequirement" {
			if requirement.DockerPull != "" {
				// NOTE: Shenglai made comment about adding `sha256` tag in order to pull exactly the latest image you want
				// ----- ask for detail/example and ask others to see if I should implement that
				return string(requirement.DockerPull)
			}
		}
	}
	return "ubuntu"
}

// get path to bash.. it is problematic to have to deal with this
// only doing this right now temporarily so that test workflow runs
// HERE TODO: come up with a better solution for this
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
