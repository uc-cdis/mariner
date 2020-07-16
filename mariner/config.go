package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
)

// this file contains type definitions for the config struct and a function for loading the config
// also defines useful/needed vars and constants

// define any needed/useful vars and consts here
const (
	// mariner components
	marinerTask   = "task"
	marinerEngine = "engine"
	s3sidecar     = "s3sidecar"
	gen3fuse      = "gen3fuse"

	// default task docker image
	// this should be in the external config
	// not in the codebase
	defaultTaskContainerImage = "ubuntu"

	// volume names
	engineWorkspaceVolumeName = "engine-workspace"
	commonsDataVolumeName     = "commons-data"
	configVolumeName          = "mariner-config"
	conformanceVolumeName     = "conformance-test"

	// location of conformance test input files in s3
	conformanceInputS3Prefix = "conformanceTest/"

	// container name
	taskContainerName = "mariner-task"

	// file path prefixes - used to differentiate COMMONS vs USER vs marinerEngine WORKSPACE file
	// user specifies commons datafile by "COMMONS/<GUID>"
	// user specifies user datafile by "USER/<path>"
	commonsPrefix     = "COMMONS/"
	userPrefix        = "USER/"
	conformancePrefix = "CONFORMANCE/"

	notStarted = "not-started" // 3
	running    = "running"     // 2
	failed     = "failed"      // 1
	completed  = "completed"   // 0
	unknown    = "unknown"
	success    = "success"
	cancelled  = "cancelled"

	k8sJobAPI     = "k8sJobAPI"
	k8sPodAPI     = "k8sPodAPI"
	k8sMetricsAPI = "k8sMetricsAPI"

	// top-level workflow ID
	mainProcessID = "#main"

	// cwl things //
	// parameter type
	CWLNullType      = "null"
	CWLFileType      = "File"
	CWLDirectoryType = "Directory"
	// object class
	CWLWorkflow        = "Workflow"
	CWLCommandLineTool = "CommandLineTool"
	CWLExpressionTool  = "ExpressionTool"
	// requirements
	CWLInitialWorkDirRequirement = "InitialWorkDirRequirement"
	CWLResourceRequirement       = "ResourceRequirement"
	CWLDockerRequirement         = "DockerRequirement"
	CWLEnvVarRequirement         = "EnvVarRequirement"
	// add the rest ..

	// log levels
	infoLogLevel    = "INFO"
	warningLogLevel = "WARNING"
	errorLogLevel   = "ERROR"

	// log file name
	logFile = "marinerLog.json"

	// done flag - used by engine
	doneFlag = "done"

	// workflow request file name
	requestFile = "request.json"

	// HTTP
	authHeader = "Authorization"

	// metrics collection sampling period (in seconds)
	metricsSamplingPeriod = 30

	// paths for engine
	pathToCommonsData = "/commons-data/data/by-guid/"
	pathToRunf        = "/engine-workspace/workflowRuns/%v/" // fill with runID
	pathToLogf        = pathToRunf + logFile
	pathToDonef       = pathToRunf + doneFlag
	pathToRequestf    = pathToRunf + requestFile
	pathToWorkingDirf = pathToRunf + "%v" // fill with runID

	// paths for server
	pathToUserRunsf   = "%v/workflowRuns/"                // fill with userID
	pathToUserRunLogf = pathToUserRunsf + "%v/" + logFile // fill with runID
)

var (
	trueVal                         = true
	falseVal                        = false
	mountPropagationHostToContainer = k8sv1.MountPropagationHostToContainer
	mountPropagationBidirectional   = k8sv1.MountPropagationBidirectional
	workflowVolumeList              = []string{engineWorkspaceVolumeName, commonsDataVolumeName, conformanceVolumeName}
)

// for mounting aws-user-creds secret to s3sidecar
var envVarAWSUserCreds = &k8sv1.EnvVarSource{
	SecretKeyRef: &k8sv1.SecretKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: Config.Secrets.AWSUserCreds.Name,
		},
		Key: Config.Secrets.AWSUserCreds.Key,
	},
}

var envVarHostname = &k8sv1.EnvVarSource{
	ConfigMapKeyRef: &k8sv1.ConfigMapKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: "global",
		},
		Key:      "hostname",
		Optional: &falseVal,
	},
}

var s3PrestopHook = &k8sv1.Lifecycle{
	PreStop: &k8sv1.Handler{
		Exec: &k8sv1.ExecAction{
			Command: Config.Containers.S3sidecar.Lifecycle.Prestop,
		},
	},
}

// could put in manifest
var gen3fusePrestopHook = &k8sv1.Lifecycle{
	PreStop: &k8sv1.Handler{
		Exec: &k8sv1.ExecAction{
			Command: Config.Containers.Gen3fuse.Lifecycle.Prestop,
		},
	},
}

// only using jupyter asg for now - will have workflow asg in production
// FIXME - put this in the manifest config
var k8sTolerations = []k8sv1.Toleration{
	{
		Key:      "role",
		Value:    "jupyter",
		Operator: k8sv1.TolerationOpEqual,
		Effect:   k8sv1.TaintEffectNoSchedule,
	},
}

type MarinerConfig struct {
	Containers Containers `json:"containers"`
	Jobs       Jobs       `json:"jobs"`
	Secrets    Secrets    `json:"secrets"`
	Storage    Storage    `json:"storage"`
}

type Storage struct {
	S3 S3Config `json:"s3"`
}

type S3Config struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}

type Containers struct {
	Engine    Container `json:"engine"`
	S3sidecar Container `json:"s3sidecar"`
	Task      Container `json:"task"`
	Gen3fuse  Container `json:"gen3fusesidecar"`
}

type Container struct {
	Name            string          `json:"name"`
	Image           string          `json:"image"`
	PullPolicy      string          `json:"pull_policy"`
	Command         []string        `json:"command"`
	Lifecycle       Lifecycle       `json:"lifecycle"`
	SecurityContext SecurityContext `json:"securitycontext"`
	Resources       Resources       `json:"resources"`
}

type Lifecycle struct {
	Prestop []string `json:"prestop"`
}

type Resources struct {
	Limits   Resource `json:"limits"`
	Requests Resource `json:"requests"`
}

type Resource struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// run as user? run as group? should mariner have those settings?
type SecurityContext struct {
	Privileged bool `json:"privileged"`
}

type Jobs struct {
	Engine JobConfig `json:"engine"`
	Task   JobConfig `json:"task"`
}

type JobConfig struct {
	Labels         map[string]string `json:"labels"`
	ServiceAccount string            `json:"serviceaccount"`
	RestartPolicy  string            `json:"restart_policy"`
}

type Secrets struct {
	AWSUserCreds *AWSUserCreds `json:"awsusercreds"`
}

type AWSUserCreds struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

func (config *MarinerConfig) jobConfig(component string) (jobConfig JobConfig) {
	switch component {
	case marinerEngine:
		jobConfig = config.Jobs.Engine
	case marinerTask:
		jobConfig = config.Jobs.Task
	}
	return jobConfig
}

func (conf *Container) resourceRequirements() (requirements k8sv1.ResourceRequirements) {
	requests, limits := make(k8sv1.ResourceList), make(k8sv1.ResourceList)
	if conf.Resources.Limits.CPU != "" {
		limits[k8sv1.ResourceCPU] = k8sResource.MustParse(conf.Resources.Limits.CPU)
	}
	if conf.Resources.Limits.Memory != "" {
		limits[k8sv1.ResourceMemory] = k8sResource.MustParse(conf.Resources.Limits.Memory)
	}
	/*
		if conf.Resources.Requests.CPU != "" {
			requests[k8sv1.ResourceCPU] = k8sResource.MustParse(conf.Resources.Requests.CPU)
		}
		if conf.Resources.Requests.Memory != "" {
			requests[k8sv1.ResourceMemory] = k8sResource.MustParse(conf.Resources.Requests.Memory)
		}
	*/
	if len(limits) > 0 {
		requirements.Limits = limits
	}
	if len(requests) > 0 {
		requirements.Requests = requests
	}
	return requirements
}

func (conf *Container) pullPolicy() (policy k8sv1.PullPolicy) {
	switch conf.PullPolicy {
	case "always":
		policy = k8sv1.PullAlways
	case "if_not_present":
		policy = k8sv1.PullIfNotPresent
	case "never":
		policy = k8sv1.PullNever
	}
	return policy
}

func (conf *Container) securityContext() (context *k8sv1.SecurityContext) {
	context = &k8sv1.SecurityContext{
		Privileged: &conf.SecurityContext.Privileged,
	}
	return context
}

func volumeMounts(component string) (v []k8sv1.VolumeMount) {
	switch component {
	case marinerEngine, marinerTask:
		v = mainVolumeMounts(component)
	case s3sidecar, gen3fuse:
		v = sidecarVolumeMounts(component)
	}
	return v
}

func sidecarVolumeMounts(component string) (v []k8sv1.VolumeMount) {
	engineWorkspace := volumeMount(engineWorkspaceVolumeName, component)
	conformanceMount := volumeMount(conformanceVolumeName, component)
	v = []k8sv1.VolumeMount{*engineWorkspace, *conformanceMount}
	if component == gen3fuse {
		commonsData := volumeMount(commonsDataVolumeName, component)
		v = append(v, *commonsData)
	}
	return v
}

func mainVolumeMounts(component string) (volumeMounts []k8sv1.VolumeMount) {
	for _, v := range workflowVolumeList {
		volumeMount := volumeMount(v, component)
		volumeMounts = append(volumeMounts, *volumeMount)
	}
	if component == marinerEngine {
		configVol := volumeMount(configVolumeName, component)
		volumeMounts = append(volumeMounts, *configVol)
	}
	return volumeMounts
}

func configVolume() *k8sv1.Volume {
	// mariner config in manifest
	// gets mounted as a volume in this way
	configMap := &k8sv1.Volume{Name: "mariner-config"}
	configMap.ConfigMap = new(k8sv1.ConfigMapVolumeSource)
	configMap.ConfigMap.Name = "manifest-mariner"
	configMap.ConfigMap.Items = []k8sv1.KeyToPath{{Key: "json", Path: "mariner-config.json"}}

	return configMap
}

func volumeMount(name string, component string) *k8sv1.VolumeMount {
	volMnt := &k8sv1.VolumeMount{
		Name:      name,
		MountPath: fmt.Sprintf("/%v", name),
	}
	switch component {
	case marinerTask, marinerEngine:
		volMnt.MountPropagation = &mountPropagationHostToContainer
	case s3sidecar, gen3fuse:
		volMnt.MountPropagation = &mountPropagationBidirectional
	}
	if name == engineWorkspaceVolumeName {
		volMnt.ReadOnly = falseVal
	}
	if name == conformanceVolumeName {
		volMnt.ReadOnly = trueVal
	}
	return volMnt
}

func (conf *JobConfig) restartPolicy() (policy k8sv1.RestartPolicy) {
	switch conf.RestartPolicy {
	case "never":
		policy = k8sv1.RestartPolicyNever
	case "on_failure":
		policy = k8sv1.RestartPolicyOnFailure
	case "always":
		policy = k8sv1.RestartPolicyAlways
	}
	return policy
}

// read `mariner-config.json` from configmap `mariner-config`
// unmarshal into go config struct FullMarinerConfig
// path is "/mariner-config/mariner-config.json"
func loadConfig(path string) (marinerConfig *MarinerConfig) {
	config, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("ERROR reading in config: %v", err)
		// log
	}
	err = json.Unmarshal(config, &marinerConfig)
	if err != nil {
		fmt.Printf("ERROR unmarshalling config into MarinerConfig struct: %v", err)
		// log
	}
	return marinerConfig
}
