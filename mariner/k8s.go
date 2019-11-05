package mariner

import (
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
// where the engine runs the workflow

////// ENGINE -> //////

// returns fully populated job spec for the workflow job (i.e, an instance of mariner-engine)
func workflowJob(workflowRequest *WorkflowRequest) (*batchv1.Job, string, error) {
	// presently this is just a timestamp - unique key is the pair (userID, runID)
	runID := runID()

	// get job spec all populated except for pod volumes and containers
	// NOTE: FIXME - job names (task and engine) are only unique within-user, not globally (among-users) unique - need to fix
	// actually task job names are not unique within-user either
	// currently the only unique job names are engine within-user
	// this needs to be fixed
	workflowJob := jobSpec(ENGINE, runID)
	workflowRequest.JobName = workflowJob.GetName()

	// fill in the rest of the spec
	workflowJob.Spec.Template.Spec.Volumes = engineVolumes()

	// runID (timestamp) is generated here! can just generate it at the very beginning of this function
	// can use it to name the job
	workflowJob.Spec.Template.Spec.Containers = engineContainers(workflowRequest, runID)
	return workflowJob, runID, nil
}

// NOTE: probably can come up with a better ID for a workflow, but for now this will work
// can't really generate a workflow ID from the given packed workflow since the top level workflow is always called "#main"
// so not exactly sure how to label the workflow runs besides a timestamp
func runID() (timeStamp string) {
	now := time.Now()
	timeStamp = fmt.Sprintf("%v-%v-%v-%v-%v-%v", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	return timeStamp
}

// returns volumes field for workflow/engine job spec
func engineVolumes() (volumes []k8sv1.Volume) {
	volumes = workflowVolumes()
	configMap := configVolume()
	volumes = append(volumes, *configMap)
	return volumes
}

func engineContainers(workflowRequest *WorkflowRequest, runID string) (containers []k8sv1.Container) {
	engine := engineContainer(runID)
	s3sidecar := s3SidecarContainer(workflowRequest, runID)
	gen3fuse := gen3fuseContainer(&workflowRequest.Manifest, ENGINE, runID)
	containers = []k8sv1.Container{*engine, *s3sidecar, *gen3fuse}
	return containers
}

func engineContainer(runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.Engine, ENGINE)
	container.Env = engineEnv(runID)
	container.Args = engineArgs(runID) // FIXME - TODO - put this in a bash script
	return container
}

// for ENGINE job
func s3SidecarContainer(request *WorkflowRequest, runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.S3sidecar, S3SIDECAR)
	container.Lifecycle = S3_PRESTOP
	container.Env = s3SidecarEnv(request, runID) // for ENGINE-sidecar
	return container
}

// given a manifest, returns the complete gen3fuse container spec for k8s podSpec
func gen3fuseContainer(manifest *Manifest, component string, runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.Gen3fuse, GEN3FUSE)
	container.Lifecycle = GEN3FUSE_PRESTOP
	container.Env = gen3fuseEnv(manifest, component, runID)
	return container
}

func gen3fuseEnv(m *Manifest, component string, runID string) (env []k8sv1.EnvVar) {
	manifest := struct2String(m)
	env = []k8sv1.EnvVar{
		{
			Name:  "GEN3_NAMESPACE",
			Value: os.Getenv("GEN3_NAMESPACE"),
		},
		{
			Name:  "ENGINE_WORKSPACE",
			Value: ENGINE_WORKSPACE,
		},
		{
			Name:  "RUN_ID",
			Value: runID,
		},
		{
			Name:  "COMMONS_DATA",
			Value: COMMONS_DATA,
		},
		{
			Name:  "MARINER_COMPONENT",
			Value: component,
		},
		{
			Name:  "GEN3FUSE_MANIFEST",
			Value: manifest,
		},
		{
			Name:      "HOSTNAME",
			ValueFrom: envVar_HOSTNAME,
		},
	}
	return env
}

func engineEnv(runID string) (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{
		{
			Name:  "GEN3_NAMESPACE",
			Value: os.Getenv("GEN3_NAMESPACE"),
		},
		{
			Name:  "ENGINE_WORKSPACE",
			Value: ENGINE_WORKSPACE,
		},
		{
			Name:  "RUN_ID",
			Value: runID,
		},
	}
	return env
}

// for ENGINE job
func s3SidecarEnv(r *WorkflowRequest, runID string) (env []k8sv1.EnvVar) {
	workflowRequest := struct2String(r)
	env = []k8sv1.EnvVar{
		{
			Name:      "AWSCREDS",
			ValueFrom: envVar_AWSCREDS,
		},
		{
			Name:  "RUN_ID",
			Value: runID,
		},
		{
			Name:  "USER_ID",
			Value: r.UserID,
		},
		{
			Name:  "MARINER_COMPONENT",
			Value: ENGINE,
		},
		{
			Name:  "WORKFLOW_REQUEST", // body of POST http request made to api
			Value: workflowRequest,
		},
		{
			Name:  "ENGINE_WORKSPACE",
			Value: ENGINE_WORKSPACE,
		},
		{
			Name:  "S3_BUCKET_NAME",
			Value: Config.Storage.S3.Name,
		},
	}
	return env
}

type TokenPayload struct {
	Context TokenContext `json:"context"`
}

type TokenContext struct {
	User TokenUser `json:"user"`
}

type TokenUser struct {
	Name string `json:"name"`
}

// FIXME - TODO - put it in a bash script
// also - why am I passing the request to the s3sidecar container?
// seems like I can just pass the request directly to the engine
// and just write some empty "done" flag in the engine workspace
// ---> to indicate the sidecar is done setting up and the engine container can run
func engineArgs(runID string) []string {
	args := []string{
		"-c",
		fmt.Sprintf(`
    while [[ ! -f /$ENGINE_WORKSPACE/workflowRuns/$RUN_ID/request.json ]]; do
      echo "Waiting for mariner-engine-sidecar to finish setting up..";
      sleep 1
    done
		echo "Sidecar setup complete! Running mariner-engine now.."
		/mariner run $RUN_ID
		`),
	}
	return args
}

////// TASK -> ///////

func (engine *K8sEngine) taskJob(tool *Tool) (job *batchv1.Job, err error) {
	jobName := tool.jobName()
	tool.JobName = jobName
	job = jobSpec(TASK, jobName)
	job.Spec.Template.Spec.Volumes = workflowVolumes()
	job.Spec.Template.Spec.Containers, err = engine.taskContainers(tool)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (engine *K8sEngine) taskContainers(tool *Tool) (containers []k8sv1.Container, err error) {
	task, err := tool.taskContainer()
	if err != nil {
		return nil, err
	}
	s3sidecar := engine.s3SidecarContainer(tool)
	gen3fuse := gen3fuseContainer(engine.Manifest, TASK, engine.RunID)
	workingDir := k8sv1.EnvVar{
		Name:  "TOOL_WORKING_DIR",
		Value: tool.WorkingDir,
	}
	gen3fuse.Env = append(gen3fuse.Env, workingDir)
	containers = []k8sv1.Container{*task, *s3sidecar, *gen3fuse}
	return containers, nil
}

// for TASK job
func (engine *K8sEngine) s3SidecarContainer(tool *Tool) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.S3sidecar, S3SIDECAR)
	container.Lifecycle = S3_PRESTOP
	container.Env = engine.s3SidecarEnv(tool)
	return container
}

// FIXME - TODO - insert some error/warning handling here
// in case errors/warnings creating the container as specified in the cwl
// additionally, add logic to check if the tool has specified each field
// if a field is not specified, the spec should be filled out using values from the mariner-config
func (tool *Tool) taskContainer() (container *k8sv1.Container, err error) {
	conf := Config.Containers.Task
	container = new(k8sv1.Container)
	container.Name = conf.Name
	container.VolumeMounts = volumeMounts(TASK)
	container.ImagePullPolicy = conf.pullPolicy()

	// if not specified use config
	container.Image = tool.dockerImage()

	// if not specified use config
	container.Resources = tool.resourceReqs()

	// if not specified use config
	container.Command = []string{tool.cltBash()} // FIXME - please

	container.Args = tool.cltArgs() // FIXME - make string constant or something

	container.Env = tool.env()

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
func (tool *Tool) cltArgs() []string {
	// Uncomment after debugging
	/*
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
					`, tool.WorkingDir, tool.WorkingDir, tool.WorkingDir, tool.cltBash(), tool.WorkingDir, tool.WorkingDir),
		}
	*/

	// for debugging
	args := []string{
		"-c",
		fmt.Sprintf(`
					while [[ ! -f %vrun.sh ]]; do
					      echo "Waiting for sidecar to finish setting up..";
					      sleep 5
					done
					echo "side done setting up"
					echo "staying alive"
					while true; do
						:
					done
					`, tool.WorkingDir),
	}

	return args
}

// env for commandlinetool
// handle EnvVarRequirement if specified - need to test
// see: https://godoc.org/k8s.io/api/core/v1#Container
// and: https://godoc.org/k8s.io/api/core/v1#EnvVar
// and: https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/
func (tool *Tool) env() (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{}
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == "EnvVarRequirement" {
			for _, envDef := range requirement.EnvDef {
				varValue, err := tool.resolveExpressions(envDef.Value) // resolves any expression(s) - if no expressions, returns original text
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
func (engine *K8sEngine) s3SidecarEnv(tool *Tool) (env []k8sv1.EnvVar) {
	env = []k8sv1.EnvVar{
		{
			Name:      "AWSCREDS",
			ValueFrom: envVar_AWSCREDS,
		},
		{
			Name:  "USER_ID",
			Value: engine.UserID,
		},
		{
			Name:  "RUN_ID",
			Value: engine.RunID,
		},
		{
			Name:  "MARINER_COMPONENT", // flag to tell setup sidecar script this is a task, not an engine job
			Value: TASK,
		},
		{
			Name:  "TOOL_COMMAND", // the command from the commandlinetool to actually execute
			Value: strings.Join(tool.Command.Args, " "),
		},
		{
			Name:  "TOOL_WORKING_DIR", // the tool's working directory - e.g., '/engine-workspace/workflowRuns/{runID}/{taskID}/'
			Value: tool.WorkingDir,
		},
		{
			Name:  "ENGINE_WORKSPACE",
			Value: ENGINE_WORKSPACE,
		},
		{
			Name:  "S3_BUCKET_NAME",
			Value: Config.Storage.S3.Name,
		},
	}
	return env
}

// for TASK job
// replace disallowed job name characters
// Q: is there a better job-naming scheme?
// -- should every mariner task job have `mariner` as a prefix, for easy identification?
func (tool *Tool) jobName() string {
	taskID := tool.Task.Root.ID
	jobName := strings.ReplaceAll(taskID, "#", "")
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ToLower(jobName)
	if tool.Task.ScatterIndex != 0 {
		// indicates this task is a scattered subtask of a task which was scattered
		// in order to not dupliate k8s job names - append suffix with ScatterIndex to job name
		jobName = fmt.Sprintf("%v-scattered-%v", jobName, tool.Task.ScatterIndex)
	}
	return jobName
}

// handles the DockerRequirement if specified and returns the image to be used for the CommandLineTool
// NOTE: if no image specified, returns `ubuntu` as a default image - need to ask/check if there is a better default image to use
// NOTE: presently only supporting use of the `dockerPull` CWL field
// FIXME
func (tool *Tool) dockerImage() string {
	for _, requirement := range tool.Task.Root.Requirements {
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
func (tool *Tool) cltBash() string {
	if tool.dockerImage() == "alpine" {
		return "/bin/sh"
	}
	return "/bin/bash"
}

// only set limits when they are specified in the CWL
// see: https://godoc.org/k8s.io/api/core/v1#Container
// the `Resources` field
// for k8s resource info see: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
//
// NOTE: presently only supporting req's for cpu cores and RAM - need to implement outdir and tmpdir and whatever other fields are allowed
func (tool *Tool) resourceReqs() k8sv1.ResourceRequirements {
	var cpuReq, cpuLim int64
	var memReq, memLim int64
	requests, limits := make(k8sv1.ResourceList), make(k8sv1.ResourceList)
	for _, requirement := range tool.Task.Root.Requirements {
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
	resourceReqs := Config.Containers.Task.resourceRequirements()

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
func baseContainer(conf *Container, component string) (container *k8sv1.Container) {
	container = &k8sv1.Container{
		Name:            conf.Name,
		Image:           conf.Image,
		Command:         conf.Command,
		ImagePullPolicy: conf.pullPolicy(),
		SecurityContext: conf.securityContext(),
		VolumeMounts:    volumeMounts(component),
		Resources:       conf.resourceRequirements(),
	}
	return container
}

// two volumes:
// 1. engine workspace
// 2. commons data
func workflowVolumes() []k8sv1.Volume {
	vols := []k8sv1.Volume{}
	for _, volName := range WORKFLOW_VOLUMES {
		vol := namedVolume(volName)
		vols = append(vols, *vol)
	}
	return vols
}

// returns ENGINE/TASK job spec with all fields populated EXCEPT volumes and containers
func jobSpec(component string, name string) (job *batchv1.Job) {

	// probably need a prefix of some kind on job names
	// some hash of the userID maybe
	// can get from token

	// if ENGINE, then `name` is the runID (i.e., timestamp)
	if component == ENGINE {
		name = fmt.Sprintf("mariner.%v", name)
	}

	jobConfig := Config.jobConfig(component)
	job = new(batchv1.Job)
	job.Kind, job.APIVersion = "Job", "v1"
	// meta for pod and job objects are same
	job.Name, job.Labels = name, jobConfig.Labels
	job.Spec.Template.Name, job.Spec.Template.Labels = name, jobConfig.Labels
	job.Spec.Template.Spec.RestartPolicy = jobConfig.restartPolicy()

	// testing - once it works, put it in the config
	// only using jupyter asg for now - will have workflow asg in production
	job.Spec.Template.Spec.Tolerations = []k8sv1.Toleration{
		k8sv1.Toleration{
			Key:      "role",
			Value:    "jupyter",
			Operator: k8sv1.TolerationOpEqual,
			Effect:   k8sv1.TaintEffectNoSchedule,
		},
	}

	if component == ENGINE {
		job.Spec.Template.Spec.ServiceAccountName = jobConfig.ServiceAccount
	}

	// HERE - TODO - get username from token, make this annotation on the pods for this workflow
	// so that workspace-token-service works
	// wts depends on this particular annotation
	job.Spec.Template.Annotations = make(map[string]string)
	job.Spec.Template.Annotations["gen3username"] = GEN3USERNAME

	return job
}

func namedVolume(name string) (v *k8sv1.Volume) {
	v = emptyVolume()
	v.Name = name
	return v
}

func emptyVolume() (v *k8sv1.Volume) {
	v = new(k8sv1.Volume)
	v.EmptyDir = &k8sv1.EmptyDirVolumeSource{}
	return v
}
