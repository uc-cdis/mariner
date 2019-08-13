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
)

// this file contains all the k8s details for creating job spec for mariner-engine and mariner-task jobs

// unfortunate terminology thing: the "workflow job" and the "engine job" are the same thing
// when I say "run a workflow job",
// it means to dispatch a job which runs an instance of the mariner-engine,
// where the engine "runs" the workflow

////// ENGINE -> //////

// returns fully populated job spec for the workflow job (i.e, an instance of mariner-engine)
func getWorkflowJob(request WorkflowRequest) (workflowJob *batchv1.Job, err error) {
	// get job spec all populated except for pod volumes and containers
	workflowJob = getJobSpec(ENGINE, "test-workflow") // FIXME - define jobname for a workflow - same as S3Prefix, or - timestamp, or

	// fill in the rest of the spec
	workflowJob.Spec.Template.Spec.Volumes = getEngineVolumes()
	workflowJob.Spec.Template.Spec.Containers = getEngineContainers(request)

	return workflowJob, nil
}

// returns volumes field for workflow/engine job spec
func getEngineVolumes() (volumes []k8sv1.Volume) {
	// the s3 bucket `workflow-engine-garvin` gets mounted in this volume
	// which is why the volume is  initialized as an empty directory
	workflowBucket := getWorkflowBucketVolume()

	// `mariner-config.json` is a configmap object (named `mariner-config` with key `config`) in the cluster
	// gets mounted as a volume in this way
	configMap := &k8sv1.Volume{Name: "mariner-config"}
	configMap.ConfigMap = new(k8sv1.ConfigMapVolumeSource)
	configMap.ConfigMap.Name = "mariner-config"
	configMap.ConfigMap.Items = []k8sv1.KeyToPath{{Key: "config", Path: "mariner-config.json"}}

	volumes = []k8sv1.Volume{*workflowBucket, *configMap}
	return volumes
}

func getEngineContainers(request WorkflowRequest) (containers []k8sv1.Container) {
	engine := getEngineContainer()
	s3sidecar := getS3SidecarContainer(request)
	containers = []k8sv1.Container{*engine, *s3sidecar}
	return containers
}

func getEngineContainer() (container *k8sv1.Container) {
	container = getBaseContainer(&Config.Config.Containers.Engine)
	container.Env = getEngineEnv()
	container.Args = getEngineArgs() // FIXME - TODO - put this in a bash script
	return container
}

// for ENGINE job
func getS3SidecarContainer(request WorkflowRequest) (container *k8sv1.Container) {
	container = getBaseContainer(&Config.Config.Containers.S3sidecar)
	// container.Args = S3SIDECARARGS, // don't need, because Command contains full command
	container.Env = getS3SidecarEnv(request) // for ENGINE-sidecar
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

// k8s namespace in which to dispatch jobs
func getEngineEnv() (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{
		{
			Name:  "GEN3_NAMESPACE",
			Value: os.Getenv("GEN3_NAMESPACE"),
		},
	}
	return env
}

// for ENGINE job
func getS3SidecarEnv(request WorkflowRequest) (env []k8sv1.EnvVar) {
	S3Prefix := getS3Prefix(request)
	requestJSON, _ := json.Marshal(request)
	env = []k8sv1.EnvVar{
		{
			Name:  "S3PREFIX",
			Value: S3Prefix, // see last line of mariner-engine-sidecar dockerfile -> "RUN goofys workflow-engine-garvin:$S3PREFIX /data"
		},
		{
			Name:      "AWSCREDS",
			ValueFrom: &awscreds,
		},
		{
			Name:  "MARINER_COMPONENT",
			Value: ENGINE,
		},
		{
			Name:  "WORKFLOW_REQUEST", // body of POST http request made to api
			Value: string(requestJSON),
		},
	}
	return env
}

// FIXME - TODO - put it in a bash script
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

////// TASK -> ///////

func (engine *K8sEngine) getTaskJob(proc *Process) (taskJob *batchv1.Job, err error) {
	jobName := proc.makeJobName() // slightly modified Root.ID
	proc.JobName = jobName
	taskJob = getJobSpec(TASK, jobName)
	taskJob.Spec.Template.Spec.Volumes = getTaskVolumes()
	taskJob.Spec.Template.Spec.Containers, err = engine.getTaskContainers(proc) // HERE - TODO
	if err != nil {
		return nil, err
	}
	return taskJob, nil
}

func getTaskVolumes() (volumes []k8sv1.Volume) {
	workflowBucket := getWorkflowBucketVolume()
	volumes = []k8sv1.Volume{*workflowBucket}
	return volumes
}

func (engine *K8sEngine) getTaskContainers(proc *Process) (containers []k8sv1.Container, err error) {
	task, err := proc.getTaskContainer()
	if err != nil {
		return nil, err
	}
	s3sidecar := engine.getS3SidecarContainer(proc)
	containers = []k8sv1.Container{*task, *s3sidecar}
	return containers, nil
}

// for TASK job
func (engine *K8sEngine) getS3SidecarContainer(proc *Process) (container *k8sv1.Container) {
	container = getBaseContainer(&Config.Config.Containers.S3sidecar)
	// container.Args = S3SIDECARARGS, // don't need, because Command contains full command
	container.Env = engine.getS3SidecarEnv(proc) //
	return container
}

// FIXME - TODO - insert some error/warning handling here
// in case errors/warnings creating the container as specified in the cwl
// additionally, add logic to check if the tool has specified each field
// if a field is not specified, the spec should be filled out using values from the mariner-config
func (proc *Process) getTaskContainer() (container *k8sv1.Container, err error) {
	conf := Config.Config.Containers.Task
	container = new(k8sv1.Container)
	container.Name = conf.Name
	container.VolumeMounts = conf.getVolumeMounts()
	container.ImagePullPolicy = conf.getPullPolicy()

	// if not specified use config
	container.Image = proc.getDockerImage()

	// if not specified use config
	container.Resources = proc.getResourceReqs()

	// if not specified use config
	container.Command = []string{proc.getCLTBash()} // FIXME - please

	container.Args = proc.getCLToolArgs() // FIXME - make string constant or something

	container.Env = proc.getEnv()

	return container, nil
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

// env for commandlinetool
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

// for TASK job
func (engine *K8sEngine) getS3SidecarEnv(proc *Process) (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{
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
			Value: TASK,
		},
		{
			Name:  "TOOL_COMMAND", // the command from the commandlinetool to actually execute
			Value: strings.Join(proc.Tool.Command.Args, " "),
		},
		{
			Name:  "TOOL_WORKING_DIR", // the tool's working directory - e.g., /data/task_id
			Value: proc.Tool.WorkingDir,
		},
	}
	return env
}

// replace disallowed job name characters
// Q: is there a better job-naming scheme?
// -- should every mariner task job have `mariner` as a prefix, for easy identification?
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
// FIXME
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

// FIXME
func (proc *Process) getCLTBash() string {
	if proc.getDockerImage() == "alpine" {
		return "/bin/sh"
	}
	return "/bin/bash"
}

// only set limits when they are specified in the CWL
// Monday
// the "default" limits are no limits - FIXME - TODO - set default "fallback" limits - maybe also vet workflows ahead of time excessive resource demands/requirements
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

	// start with default settings
	resourceReqs := Config.Config.Containers.Task.getResourceRequirements()

	// only want to overwrite default limits if requirements specified in the CWL
	if len(requests) > 0 {
		resourceReqs.Requests = requests
	}
	if len(limits) > 0 {
		resourceReqs.Limits = limits
	}
	return resourceReqs
}

/////// General purpose - for TASK & ENGINE -> ///////

// for info, see: https://godoc.org/k8s.io/api/core/v1#Container
func getBaseContainer(conf *Container) (container *k8sv1.Container) {
	container = &k8sv1.Container{
		Name:            conf.Name,
		Image:           conf.Image,
		Command:         conf.Command,
		ImagePullPolicy: conf.getPullPolicy(),
		SecurityContext: conf.getSecurityContext(),
		VolumeMounts:    conf.getVolumeMounts(),
		Resources:       conf.getResourceRequirements(),
	}
	return container
}

// returns ENGINE/TASK job spec with all fields populated EXCEPT volumes and containers
func getJobSpec(component string, name string) (job *batchv1.Job) {
	jobConfig := Config.getJobConfig(component)

	job = new(batchv1.Job)
	// job.TypeMeta = metav1.TypeMeta{Kind: "Job", APIVersion: "v1"}
	job.Kind, job.APIVersion = "Job", "v1"
	job.Name, job.Labels = name, jobConfig.Labels

	// objectMeta := metav1.ObjectMeta{Name: "REPLACEME", Labels: jobConfig.Labels} // TODO - make jobname a parameter
	// job.ObjectMeta, job.Spec.Template.ObjectMeta = objectMeta, objectMeta        // meta for pod and job objects are same
	job.Spec.Template.Name, job.Spec.Template.Labels = "REPLACEME", jobConfig.Labels
	job.Spec.Template.Spec.RestartPolicy = jobConfig.getRestartPolicy()
	if component == ENGINE {
		job.Spec.Template.Spec.ServiceAccountName = jobConfig.ServiceAccount
	}
	return job
}

func getWorkflowBucketVolume() (v *k8sv1.Volume) {
	v = getEmptyVolume()
	v.Name = "shared-data"
	return v
}

func getEmptyVolume() (v *k8sv1.Volume) {
	v = new(k8sv1.Volume)
	v.EmptyDir = &k8sv1.EmptyDirVolumeSource{}
	return v
}
