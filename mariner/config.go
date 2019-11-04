package mariner

import (
	"encoding/json"
	"fmt"
	// "fmt"
	"io/ioutil"
	// "os"
	// "time"

	k8sv1 "k8s.io/api/core/v1"
	k8sResource "k8s.io/apimachinery/pkg/api/resource"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// batchv1 "k8s.io/api/batch/v1"
)

// this file contains type definitions for the config struct and a function for loading the config
// also defines useful/needed vars and constants

// define any needed/useful vars and consts here
var (
	trueVal                         = true
	falseVal                        = false
	MountPropagationHostToContainer = k8sv1.MountPropagationHostToContainer
	MountPropagationBidirectional   = k8sv1.MountPropagationBidirectional
	WORKFLOW_VOLUMES                = []string{ENGINE_WORKSPACE, COMMONS_DATA}
)

const (
	// mariner components
	TASK      = "TASK"
	ENGINE    = "ENGINE"
	S3SIDECAR = "S3SIDECAR"
	GEN3FUSE  = "GEN3FUSE"

	// volume names
	ENGINE_WORKSPACE = "engine-workspace"
	COMMONS_DATA     = "commons-data"
	CONFIG           = "mariner-config"

	// file path prefixes - used to differentiate COMMONS vs USER vs ENGINE WORKSPACE file
	// user specifies commons datafile by "COMMONS/<GUID>"
	// user specifies user datafile by "USER/<path>"
	COMMONS_PREFIX       = "COMMONS/"
	USER_PREFIX          = "USER/"
	PATH_TO_COMMONS_DATA = "/commons-data/data/by-guid/"

	// for pod annotation so that WTS works
	// only here for testing, of course
	GEN3USERNAME = "mgarvin3@uchicago.edu"

	NOT_STARTED = "not-started" // 3
	RUNNING     = "running"     // 2
	FAILED      = "failed"      // 1
	COMPLETED   = "completed"   // 0
	UNKNOWN     = "unknown"
	SUCCESS     = "success"

	// log levels
	INFO    = "INFO"
	WARNING = "WARNING"
	ERROR   = "ERROR"

	// HTTP
	AUTH_HEADER = "Authorization"
)

// for mounting aws-user-creds secret to s3sidecar
var envVar_AWSCREDS = &k8sv1.EnvVarSource{
	SecretKeyRef: &k8sv1.SecretKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: Config.Secrets.AWSUserCreds.Name,
		},
		Key: Config.Secrets.AWSUserCreds.Key,
	},
}

var envVar_HOSTNAME = &k8sv1.EnvVarSource{
	ConfigMapKeyRef: &k8sv1.ConfigMapKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: "global",
		},
		Key:      "hostname",
		Optional: &falseVal,
	},
}

// could put in manifest
var S3_PRESTOP = &k8sv1.Lifecycle{
	PreStop: &k8sv1.Handler{
		Exec: &k8sv1.ExecAction{
			Command: []string{"fusermount", "-u", "-z", "/$ENGINE_WORKSPACE"},
		},
	},
}

// could put in manifest
var GEN3FUSE_PRESTOP = &k8sv1.Lifecycle{
	PreStop: &k8sv1.Handler{
		Exec: &k8sv1.ExecAction{
			Command: []string{"fusermount", "-u", "/$COMMONS_DATA"},
		},
	},
}

type MarinerConfig struct {
	Containers Containers `json:"containers"`
	Jobs       Jobs       `json:"jobs"`
	Secrets    Secrets    `json:"secrets"`
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
	SecurityContext SecurityContext `json:"securitycontext"`
	Resources       Resources       `json:"resources"`
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
	AWSUserCreds AWSUserCreds `json:"awsusercreds"`
}

type AWSUserCreds struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

func (config *MarinerConfig) jobConfig(component string) (jobConfig JobConfig) {
	switch component {
	case ENGINE:
		jobConfig = config.Jobs.Engine
	case TASK:
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
	case ENGINE, TASK:
		v = mainVolumeMounts(component)
	case S3SIDECAR, GEN3FUSE:
		v = sidecarVolumeMounts(component)
	}
	return v
}

func sidecarVolumeMounts(component string) (v []k8sv1.VolumeMount) {
	engineWorkspace := volumeMount(ENGINE_WORKSPACE, component)
	v = []k8sv1.VolumeMount{*engineWorkspace}
	if component == GEN3FUSE {
		commonsData := volumeMount(COMMONS_DATA, component)
		v = append(v, *commonsData)
	}
	return v
}

func mainVolumeMounts(component string) (volumeMounts []k8sv1.VolumeMount) {
	for _, v := range WORKFLOW_VOLUMES {
		volumeMount := volumeMount(v, component)
		volumeMounts = append(volumeMounts, *volumeMount)
	}
	if component == ENGINE {
		configVol := volumeMount(CONFIG, component)
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
		// ReadOnly:  trueVal,
	}
	switch component {
	case TASK, ENGINE:
		volMnt.MountPropagation = &MountPropagationHostToContainer
	case S3SIDECAR, GEN3FUSE:
		volMnt.MountPropagation = &MountPropagationBidirectional
	}
	// all vols are readOnly except the engine workspace
	// that is, all files generated by a task are written/stored to engine workspace
	if name == ENGINE_WORKSPACE {
		volMnt.ReadOnly = falseVal
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
	}
	err = json.Unmarshal(config, &marinerConfig)
	if err != nil {
		fmt.Printf("ERROR unmarshalling config into MarinerConfig struct: %v", err)
	}
	return marinerConfig
}
