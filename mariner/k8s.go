package mariner

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"os"

	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// this file contains all the k8s details for creating job spec for mariner-engine and mariner-task jobs

// returns fully populated job spec for the workflow job (i.e, an instance of mariner-engine)
func getWorkflowJob(request WorkflowRequest) (workflowJob *batchv1.Job, err error) {
	// get job spec all populated except for pod volumes and containers
	workflowJob = getJobSpec(ENGINE)

	// fill in the rest of the spec
	workflowJob.Spec.Template.Spec.Volumes = getEngineVolumes()
	workflowJob.Spec.Template.Spec.Containers = getEngineContainers(request)

	return workflowJob, nil
}

// returns volumes field for workflow/engine job spec
func getEngineVolumes() (volumes []k8sv1.Volume) {
	// the s3 bucket `workflow-engine-garvin` gets mounted in this volume
	// which is why the volume is  initialized as an empty directory
	workflowBucket := k8sv1.Volume{Name: "shared-data"}
	workflowBucket.EmptyDir = &k8sv1.EmptyDirVolumeSource{}

	// `mariner-config.json` is a configmap object in the cluster
	// gets mounted as a volume in this way
	configMap :=  k8sv1.Volume{Name: "mariner-config"}
	configMap.ConfigMap.Name = "mariner-config"
	configMap.ConfigMap.Items = []k8sv1.KeyToPath{{Key: "config", Path: "mariner.json"}}

	volumes = []k8sv1.Volume{workflowBucket, configMap}
	return volumes
}

// HERE - TODO - add sensible resource requirements here - ask devops
func getEngineContainers(request WorkflowRequest) (containers []k8sv1.Container) {
	engine := getEngineContainer()
	s3sidecar := getS3SidecarContainer(request)
	containers = []k8sv1.Container{*engine, *s3sidecar}
	return containers
}

func getEngineContainer() (container *k8sv1.Container) {
	container = getBaseContainer(&Config.Config.Containers.Engine)
	container.Env = getEngineEnv()
	container.Args = getEngineArgs() // TODO - put this in a bash script
	return container
}

func getS3SidecarContainer(request WorkflowRequest) (container *k8sv1.Container) {
	container = getBaseContainer(&Config.Config.Containers.S3sidecar)
	// container.Args = S3SIDECARARGS, // don't need, because Command contains full command
	container.Env = getS3SidecarEnv(request) // for ENGINE-sidecar
	return container
}

func getBaseContainer(conf *Container) (container *k8sv1.Container) {
	container = &k8sv1.Container{
		Name:            conf.Name,
		Image:           conf.Image,
		Command:         conf.Command,
		ImagePullPolicy: conf.getPullPolicy(),
		SecurityContext: conf.getSecurityContext(),
		VolumeMounts:    conf.getVolumeMounts(),
	}
	return container
}

// NOTE: probably can come up with a better ID for a workflow, but for now this will work
// can't really generate a workflow ID from the given packed workflow since the top level workflow is always called "#main"
// so not exactly sure how to label the workflow runs besides a timestamp
func getS3Prefix(request WorkflowRequest) (prefix string) {
	now := time.Now()
	timeStamp := fmt.Sprintf("%v-%v-%v_%v-%v-%v", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	prefix = fmt.Sprintf("/%v/%v/", request.ID, timeStamp)
	return prefix
}

func getEngineEnv() (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{
		{
			Name:  "GEN3_NAMESPACE",
			Value: os.Getenv("GEN3_NAMESPACE"),
		},
	}
	return env
}

func getS3SidecarEnv(request WorkflowRequest) (env []k8sv1.EnvVar) {
	S3Prefix := getS3Prefix(request)
	requestJSON, _ := json.Marshal(request)
	env = []k8sv1.EnvVar{
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
			Value: ENGINE,
		},
		{
			Name:  "WORKFLOW_REQUEST", // in proprocessing body of POST http request made to api
			Value: string(requestJSON),
		},
	}
	return env
}

// returns ENGINE/TASK job spec with all fields populated EXCEPT volumes and containers
func getJobSpec(component string) (job *batchv1.Job) {
	jobConfig := Config.getJobConfig(component)
	job.TypeMeta = metav1.TypeMeta{Kind: "Job", APIVersion: "v1"}
	objectMeta := metav1.ObjectMeta{Name: "REPLACEME", Labels: jobConfig.Labels} // TODO - make jobname a parameter
	job.ObjectMeta, job.Spec.Template.ObjectMeta = objectMeta, objectMeta // meta for pod and job objects are same
	job.Spec.Template.Spec.RestartPolicy = jobConfig.getRestartPolicy()
	if component == ENGINE {
		job.Spec.Template.Spec.ServiceAccountName = jobConfig.ServiceAccount
	}
	return job
}

// analogous to task main container args - maybe restructure code, less redundancy
// TODO - put it in a bash script
func getEngineArgs() []string {
	args := []string{
		"-c",
		fmt.Sprintf(`
    while [[ ! -f /data/request.json ]]; do
      echo "Waiting for mariner-engine-sidecar to finish setting up..";
      sleep 1
    done
		echo "Sidecar setup complete! Running mariner-engine now.."
		/mariner run $S3PREFIX
		`),
	}
	return args
}

// wait for sidecar to setup
// in particular wait until run.sh exists (run.sh is the command for the Tool)
// as soon as run.sh exists, run this script
// HERE TODO - put this in a bash script
// actually don't, because the CLT runs in its own container
// - won't have the mariner repo, and we shouldn't clone it in there
// so, just make this string a constant or something in the config file
// TOOL_WORKING_DIR is an envVar - no need to inject from go vars here
// HERE - how to handle case of different possible bash, depending on CLT image specified in CWL?
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
// HERE - Tuesday
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
									MountPropagation: &MountPropagationHostToContainer,
								},
							},
						},
						// make method for engine - engine.getS3SidecarContainer(tool) or something
						{
							Name:  "mariner-s3sidecar",
							Image: "quay.io/cdis/mariner-s3sidecar:feat_k8s", // put in config
							Command: []string{
								"/bin/sh",
							},
							// Args:            S3SIDECARARGS, // calls bash setup script - see envVars for vars referenced in the script
							ImagePullPolicy: k8sv1.PullPolicy(k8sv1.PullAlways),
							SecurityContext: &k8sv1.SecurityContext{
								Privileged: &trueVal,
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
									Value: TASK,                // put this in config, don't hardcode it here - also potentially use different flag name
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
									MountPropagation: &MountPropagationBidirectional,
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
