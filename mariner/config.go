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
	// S3SIDECARARGS                   = []string{"./s3sidecarDockerrun.sh"}
	WORKFLOW_VOLUMES = []string{ENGINE_WORKSPACE, COMMONS_DATA, USER_DATA}
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
	USER_DATA        = "user-data"
	CONFIG           = "mariner-config"

	// for pod annotation so that WTS works
	// only here for testing, of course
	GEN3USERNAME = "mgarvin3@uchicago.edu"
)

// for mounting aws-user-creds secret to s3sidecar
var awscreds = k8sv1.EnvVarSource{
	SecretKeyRef: &k8sv1.SecretKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: Config.Secrets.AWSUserCreds.Name,
		},
		Key: Config.Secrets.AWSUserCreds.Key,
	},
}

var envVar_HOSTNAME = k8sv1.EnvVar{
	Name: "HOSTNAME",
	ValueFrom: &k8sv1.EnvVarSource{
		ConfigMapKeyRef: &k8sv1.ConfigMapKeySelector{
			LocalObjectReference: k8sv1.LocalObjectReference{
				Name: "global",
			},
			Key:      "hostname",
			Optional: &falseVal,
		},
	},
}

func (config *MarinerConfig) getJobConfig(component string) (jobConfig JobConfig) {
	switch component {
	case ENGINE:
		jobConfig = config.Jobs.Engine
	case TASK:
		jobConfig = config.Jobs.Task
	}
	return jobConfig
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

func (conf *Container) getResourceRequirements() (requirements k8sv1.ResourceRequirements) {
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

func (conf *Container) getPullPolicy() (policy k8sv1.PullPolicy) {
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

func (conf *Container) getSecurityContext() (context *k8sv1.SecurityContext) {
	context = &k8sv1.SecurityContext{
		Privileged: &conf.SecurityContext.Privileged,
	}
	return context
}

func getVolumeMounts(component string) (v []k8sv1.VolumeMount) {
	switch component {
	case ENGINE, TASK:
		v = getMainVolumeMounts(component)
	case S3SIDECAR, GEN3FUSE:
		v = getSidecarVolumeMounts(component)
	}
	return v
}

func getSidecarVolumeMounts(component string) (v []k8sv1.VolumeMount) {
	engineWorkspace := getVolumeMount(ENGINE_WORKSPACE, component)
	v = []k8sv1.VolumeMount{*engineWorkspace}
	switch component {
	case GEN3FUSE:
		commonsData := getVolumeMount(COMMONS_DATA, component)
		v = append(v, *commonsData)
	case S3SIDECAR:
		userData := getVolumeMount(USER_DATA, component)
		v = append(v, *userData)
	}
	return v
}

func getMainVolumeMounts(component string) (volumeMounts []k8sv1.VolumeMount) {
	for _, v := range WORKFLOW_VOLUMES {
		volumeMount := getVolumeMount(v, component)
		volumeMounts = append(volumeMounts, *volumeMount)
	}
	if component == ENGINE {
		configVol := getVolumeMount(CONFIG, component)
		volumeMounts = append(volumeMounts, *configVol)
	}
	return volumeMounts
}

func getConfigVolume() *k8sv1.Volume {
	// mariner config in manifest
	// gets mounted as a volume in this way
	configMap := &k8sv1.Volume{Name: "mariner-config"}
	configMap.ConfigMap = new(k8sv1.ConfigMapVolumeSource)
	configMap.ConfigMap.Name = "manifest-mariner"
	configMap.ConfigMap.Items = []k8sv1.KeyToPath{{Key: "json", Path: "mariner-config.json"}}

	return configMap
}

func getVolumeMount(name string, component string) *k8sv1.VolumeMount {
	volMnt := &k8sv1.VolumeMount{
		Name:      name,
		MountPath: fmt.Sprintf("/%v", name),
		ReadOnly:  trueVal,
	}
	switch component {
	case TASK, ENGINE:
		volMnt.MountPropagation = &MountPropagationHostToContainer
	case S3SIDECAR:
		volMnt.MountPropagation = &MountPropagationBidirectional
	}
	// all vols are readOnly except the engine workspace
	// that is, all files generated by a task are written/stored to engine workspace
	if name == ENGINE_WORKSPACE {
		volMnt.ReadOnly = falseVal
	}
	return volMnt
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

func (conf *JobConfig) getRestartPolicy() (policy k8sv1.RestartPolicy) {
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

type Secrets struct {
	AWSUserCreds AWSUserCreds `json:"awsusercreds"`
}

type AWSUserCreds struct {
	Name string `json:"name"`
	Key  string `json:"key"`
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
