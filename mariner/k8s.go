package mariner

import (
	"fmt"
	"math"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
)

// this file contains code for creating job spec for mariner-engine and mariner-task jobs

// unfortunate terminology thing: the "workflow job" and the "engine job" are the same thing
// when I say "run a workflow job",
// it means to dispatch a job which runs an instance of the mariner-engine,
// where the engine runs the workflow

////// marinerEngine -> //////

// returns fully populated job spec for the workflow job (i.e, an instance of mariner-engine)
func workflowJob(workflowRequest *WorkflowRequest) (*batchv1.Job, string, error) {

	// get job spec all populated except for pod volumes and containers
	workflowJob := jobSpec(marinerEngine, workflowRequest.UserID)
	workflowRequest.JobName = workflowJob.GetName()

	// fill in the rest of the spec
	workflowJob.Spec.Template.Spec.Volumes = engineVolumes()

	workflowJob.Spec.Template.Spec.Containers = engineContainers(workflowRequest, workflowRequest.JobName)
	return workflowJob, workflowRequest.JobName, nil
}

// returns volumes field for workflow/engine job spec
func engineVolumes() (volumes []k8sv1.Volume) {
	volumes = workflowVolumes()
	configMap := configVolume()
	volumes = append(volumes, *configMap)
	return volumes
}

// `runID` is the jobName of the engine job
func engineContainers(workflowRequest *WorkflowRequest, runID string) (containers []k8sv1.Container) {
	engine := engineContainer(runID)
	s3sidecar := s3SidecarContainer(workflowRequest, runID)
	gen3fuse := gen3fuseContainer(&workflowRequest.Manifest, marinerEngine, runID)
	containers = []k8sv1.Container{*engine, *s3sidecar, *gen3fuse}
	return containers
}

func engineContainer(runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.Engine, marinerEngine)
	container.Env = engineEnv(runID)
	return container
}

// for marinerEngine job
func s3SidecarContainer(request *WorkflowRequest, runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.S3sidecar, s3sidecar)
	container.Lifecycle = s3PrestopHook
	container.Env = s3SidecarEnv(request, runID) // for marinerEngine-sidecar
	return container
}

// given a manifest, returns the complete gen3fuse container spec for k8s podSpec
func gen3fuseContainer(manifest *Manifest, component string, runID string) (container *k8sv1.Container) {
	container = baseContainer(&Config.Containers.Gen3fuse, gen3fuse)
	container.Lifecycle = gen3fusePrestopHook
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
			Value: engineWorkspaceVolumeName,
		},
		{
			Name:  "RUN_ID",
			Value: runID,
		},
		{
			Name:  "COMMONS_DATA",
			Value: commonsDataVolumeName,
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
			ValueFrom: envVarHostname,
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
			Value: engineWorkspaceVolumeName,
		},
		{
			Name:  "RUN_ID",
			Value: runID,
		},
	}
	return env
}

// for marinerEngine job
func s3SidecarEnv(r *WorkflowRequest, runID string) (env []k8sv1.EnvVar) {
	workflowRequest := struct2String(r)
	env = []k8sv1.EnvVar{
		{
			Name:      "AWSCREDS",
			ValueFrom: envVarAWSUserCreds,
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
			Value: marinerEngine,
		},
		{
			Name:  "WORKFLOW_REQUEST", // body of POST http request made to api
			Value: workflowRequest,
		},
		{
			Name:  "ENGINE_WORKSPACE",
			Value: engineWorkspaceVolumeName,
		},
		{
			Name:  "CONFORMANCE_INPUT_S3_PREFIX",
			Value: conformanceInputS3Prefix,
		},
		{
			Name:  "CONFORMANCE_INPUT_DIR",
			Value: conformanceVolumeName,
		},
		{
			Name:  "S3_BUCKET_NAME",
			Value: Config.Storage.S3.Name,
		},
		{
			Name:  "S3_REGION",
			Value: Config.Storage.S3.Region,
		},
	}

	conformanceTestFlag := k8sv1.EnvVar{
		Name: "CONFORMANCE_TEST",
	}
	if r.Tags["conformanceTest"] == "true" {
		conformanceTestFlag.Value = "true"
	} else {
		conformanceTestFlag.Value = "false"
	}

	env = append(env, conformanceTestFlag)
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

////// marinerTask -> ///////

func (engine *K8sEngine) taskJob(tool *Tool) (job *batchv1.Job, err error) {
	engine.infof("begin load job spec for task: %v", tool.Task.Root.ID)
	jobName := tool.jobName()
	tool.JobName = jobName
	job = jobSpec(marinerTask, engine.UserID)

	if engine.Log.Request.ServiceAccountName != "" {
		job.Spec.Template.Spec.ServiceAccountName = engine.Log.Request.ServiceAccountName
	}

	job.Spec.Template.Spec.Volumes = workflowVolumes()
	job.Spec.Template.Spec.Containers, err = engine.taskContainers(tool)
	if err != nil {
		return nil, engine.errorf("failed to load container spec for task: %v; error: %v", tool.Task.Root.ID, err)
	}
	engine.infof("end load job spec for task: %v", tool.Task.Root.ID)
	return job, nil
}

func (engine *K8sEngine) taskContainers(tool *Tool) (containers []k8sv1.Container, err error) {
	engine.infof("begin load container spec for tool: %v", tool.Task.Root.ID)
	task, err := tool.taskContainer()
	if err != nil {
		return nil, engine.errorf("failed to load task main container: %v; error: %v", tool.Task.Root.ID, err)
	}
	s3sidecar := engine.s3SidecarContainer(tool)
	gen3fuse := gen3fuseContainer(engine.Manifest, marinerTask, engine.RunID)
	workingDir := k8sv1.EnvVar{
		Name:  "TOOL_WORKING_DIR",
		Value: tool.WorkingDir,
	}
	gen3fuse.Env = append(gen3fuse.Env, workingDir)
	task.Env = append(task.Env, workingDir)
	containers = []k8sv1.Container{*task, *s3sidecar, *gen3fuse}
	engine.infof("end load container spec for tool: %v", tool.Task.Root.ID)
	return containers, nil
}

// for marinerTask job
func (engine *K8sEngine) s3SidecarContainer(tool *Tool) (container *k8sv1.Container) {
	engine.infof("load s3 sidecar container spec for task: %v", tool.Task.Root.ID)
	container = baseContainer(&Config.Containers.S3sidecar, s3sidecar)
	container.Lifecycle = s3PrestopHook
	container.Env = engine.s3SidecarEnv(tool)
	return container
}

// FIXME - TODO - insert some error/warning handling here
// in case errors/warnings creating the container as specified in the cwl
// additionally, add logic to check if the tool has specified each field
// if a field is not specified, the spec should be filled out using values from the mariner-config
func (tool *Tool) taskContainer() (container *k8sv1.Container, err error) {
	tool.Task.infof("begin load main container spec")
	conf := Config.Containers.Task
	container = new(k8sv1.Container)
	container.Name = conf.Name
	container.VolumeMounts = volumeMounts(marinerTask)
	container.ImagePullPolicy = conf.pullPolicy()

	container.Image = tool.dockerImage()
	tool.Task.Log.ContainerImage = container.Image

	if container.Resources, err = tool.resourceReqs(); err != nil {
		return nil, tool.Task.errorf("failed to load cpu/mem info: %v", err)
	}

	// if not specified use config
	container.Command = []string{tool.cltBash()} // fixme - please

	container.Args = tool.cltArgs() // fixme - make string constant or something

	if container.Env, err = tool.env(); err != nil {
		return nil, tool.Task.errorf("failed to load env info: %v", err)
	}

	tool.Task.infof("end load main container spec")
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
// Q: how to handle case of different possible bash, depending on CLT image specified in CWL?
// fixme
func (tool *Tool) cltArgs() []string {
	tool.Task.infof("begin load CommandLineTool container args")
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
			touch %vdone
			`, tool.WorkingDir, tool.WorkingDir, tool.WorkingDir, tool.cltBash(), tool.WorkingDir, tool.WorkingDir),
	}

	// for debugging
	/*
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
	*/

	tool.Task.infof("end load CommandLineTool container args")
	return args
}

// env for commandlinetool
// handle EnvVarRequirement if specified - need to test
// see: https://godoc.org/k8s.io/api/core/v1#Container
// and: https://godoc.org/k8s.io/api/core/v1#EnvVar
// and: https://kubernetes.io/docs/tasks/inject-data-application/define-environment-variable-container/
func (tool *Tool) env() (env []k8sv1.EnvVar, err error) {
	tool.Task.infof("begin load environment variables")
	env = []k8sv1.EnvVar{}
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == CWLEnvVarRequirement {
			for _, envDef := range requirement.EnvDef {
				tool.Task.infof("begin handle envVar: %v", envDef.Name)
				varValue, _, err := tool.resolveExpressions(envDef.Value) // resolves any expression(s) - if no expressions, returns original text
				if err != nil {
					return nil, tool.Task.errorf("failed to resolve expression: %v; error: %v", envDef.Value, err)
				}
				envVar := k8sv1.EnvVar{
					Name:  envDef.Name,
					Value: varValue,
				}
				env = append(env, envVar)
				tool.Task.infof("end handle envVar: %v", envDef.Name)
			}
		}
	}
	tool.Task.infof("end load environment variables")
	return env, nil
}

// for marinerTask job
func (engine *K8sEngine) s3SidecarEnv(tool *Tool) (env []k8sv1.EnvVar) {
	engine.infof("load s3 sidecar env for task: %v", tool.Task.Root.ID)
	env = []k8sv1.EnvVar{
		{
			Name:      "AWSCREDS",
			ValueFrom: envVarAWSUserCreds,
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
			Value: marinerTask,
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
			Value: engineWorkspaceVolumeName,
		},
		{
			Name:  "S3_BUCKET_NAME",
			Value: Config.Storage.S3.Name,
		},
		{
			Name:  "S3_REGION",
			Value: Config.Storage.S3.Region,
		},
		{
			Name:  "CONFORMANCE_INPUT_S3_PREFIX",
			Value: conformanceInputS3Prefix,
		},
		{
			Name:  "CONFORMANCE_INPUT_DIR",
			Value: conformanceVolumeName,
		},
	}

	conformanceTestFlag := k8sv1.EnvVar{
		Name: "CONFORMANCE_TEST",
	}
	if engine.Log.Request.Tags["conformanceTest"] == "true" {
		conformanceTestFlag.Value = "true"
	} else {
		conformanceTestFlag.Value = "false"
	}

	env = append(env, conformanceTestFlag)

	return env
}

// for marinerTask job
// replace disallowed job name characters
// Q: is there a better job-naming scheme?
func (tool *Tool) jobName() string {
	tool.Task.infof("begin resolve k8s job name")
	taskID := tool.Task.Root.ID
	jobName := strings.ReplaceAll(taskID, "#", "")
	jobName = strings.ReplaceAll(jobName, "_", "-")
	jobName = strings.ToLower(jobName)
	if tool.Task.ScatterIndex != 0 {
		// indicates this task is a scattered subtask of a task which was scattered
		// in order to not dupliate k8s job names - append suffix with ScatterIndex to job name
		jobName = fmt.Sprintf("%v-scattered-%v", jobName, tool.Task.ScatterIndex)
	}
	tool.Task.infof("end resolve k8s job name. resolved job name: %v", jobName)
	return jobName
}

// handles the DockerRequirement if specified and returns the image to be used for the CommandLineTool
// note: presently only supporting use of the `dockerPull` CWL field
// fixme - handle remaining DockerRequirement options
func (tool *Tool) dockerImage() string {
	tool.Task.infof("begin load docker image")
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == CWLDockerRequirement {
			if requirement.DockerPull != "" {
				tool.Task.infof("end load docker image. loaded image: %v", string(requirement.DockerPull))
				return string(requirement.DockerPull)
			}
		}
	}
	tool.Task.infof("end load docker image. loaded default task image: %v", defaultTaskContainerImage)
	return defaultTaskContainerImage
}

// fixme
// Q: how to handle case of different possible bash, depending on CLT image specified in CWL?
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
func (tool *Tool) resourceReqs() (k8sv1.ResourceRequirements, error) {
	tool.Task.infof("begin handle resource requirements")
	var cpuReq, cpuLim int64
	var memReq, memLim int64

	// start with default settings
	resourceReqs := Config.Containers.Task.resourceRequirements()

	// discern user specified settings
	requests, limits := make(k8sv1.ResourceList), make(k8sv1.ResourceList)
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == CWLResourceRequirement {
			// for info on quantities, see: https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity
			if requirement.CoresMin > 0 {
				cpuReq = int64(requirement.CoresMin)
				tool.Task.Log.Stats.CPUReq.Min = cpuReq
				requests[k8sv1.ResourceCPU] = *k8sResource.NewQuantity(cpuReq, k8sResource.DecimalSI)
			}

			if requirement.CoresMax > 0 {
				cpuLim = int64(requirement.CoresMax)
				tool.Task.Log.Stats.CPUReq.Max = cpuLim
				limits[k8sv1.ResourceCPU] = *k8sResource.NewQuantity(cpuLim, k8sResource.DecimalSI)
			}

			// Memory is provided in mebibytes (1 mebibyte is 2**20 bytes)
			// here we convert mebibytes to bytes
			if requirement.RAMMin > 0 {
				memReq = int64(requirement.RAMMin * int(math.Pow(2, 20)))
				tool.Task.Log.Stats.MemoryReq.Min = memReq
				requests[k8sv1.ResourceMemory] = *k8sResource.NewQuantity(memReq, k8sResource.DecimalSI)
			}

			if requirement.RAMMax > 0 {
				memLim = int64(requirement.RAMMax * int(math.Pow(2, 20)))
				tool.Task.Log.Stats.MemoryReq.Max = memLim
				limits[k8sv1.ResourceMemory] = *k8sResource.NewQuantity(memLim, k8sResource.DecimalSI)
			}
		}
	}

	// sanity check for negative requirements
	reqVals := []int64{cpuReq, cpuLim, memReq, memLim}
	for _, val := range reqVals {
		if val < 0 {
			return resourceReqs, tool.Task.errorf("negative memory or cores requirement specified")
		}
	}

	// verify valid bounds if both min and max specified
	if memLim > 0 && memReq > 0 && memLim < memReq {
		return resourceReqs, tool.Task.errorf("memory maximum specified less than memory minimum specified")
	}

	if cpuLim > 0 && cpuReq > 0 && cpuLim < cpuReq {
		return resourceReqs, tool.Task.errorf("cores maximum specified less than cores minimum specified")
	}

	// only overwrite default limits if requirements specified in the CWL by user
	if len(requests) > 0 {
		resourceReqs.Requests = requests
	}
	if len(limits) > 0 {
		resourceReqs.Limits = limits
	}

	tool.Task.infof("end handle resource requirements")
	return resourceReqs, nil
}

/////// General purpose - for marinerTask & marinerEngine -> ///////

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
	for _, volName := range workflowVolumeList {
		vol := namedVolume(volName)
		vols = append(vols, *vol)
	}
	return vols
}

// returns marinerEngine/marinerTask job spec with all fields populated EXCEPT volumes and containers
func jobSpec(component string, userID string) (job *batchv1.Job) {

	jobConfig := Config.jobConfig(component)
	job = new(batchv1.Job)
	job.Kind, job.APIVersion = "Job", "v1"
	// meta for pod and job objects are same
	jobName := createJobName()
	job.Name, job.Labels = jobName, jobConfig.Labels
	job.Spec.Template.Name, job.Spec.Template.Labels = jobName, jobConfig.Labels
	job.Spec.Template.Spec.RestartPolicy = jobConfig.restartPolicy()
	job.Spec.Template.Spec.Tolerations = k8sTolerations

	if component == marinerEngine {
		job.Spec.Template.Spec.ServiceAccountName = jobConfig.ServiceAccount
	}

	// wts depends on this particular annotation
	job.Spec.Template.Annotations = make(map[string]string)
	job.Spec.Template.Annotations["gen3username"] = userID

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
