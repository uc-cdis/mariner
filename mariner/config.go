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
)

const (
	TASK   = "TASK"
	ENGINE = "ENGINE"
)

// for mounting aws-user-creds secret to s3sidecar
var awscreds = k8sv1.EnvVarSource{
	SecretKeyRef: &k8sv1.SecretKeySelector{
		LocalObjectReference: k8sv1.LocalObjectReference{
			Name: Config.Config.Secrets.AWSUserCreds.Name,
		},
		Key: Config.Config.Secrets.AWSUserCreds.Key,
	},
}

type FullMarinerConfig struct {
	Config MarinerConfig `json:"mariner"`
}

func (config *FullMarinerConfig) getJobConfig(component string) (jobConfig JobConfig) {
	defer fmt.Println("in getJobConfig..")
	defer PrintJSON(&config)
	switch component {
	case ENGINE:
		jobConfig = config.Config.Jobs.Engine
	case TASK:
		jobConfig = config.Config.Jobs.Task
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
}

type Container struct {
	Name            string          `json:"name"`
	Image           string          `json:"image"`
	PullPolicy      string          `json:"pull_policy"`
	Command         []string        `json:"command"`
	VolumeMounts    []VolumeMount   `json:"volume_mounts"`
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

func (conf *Container) getVolumeMounts() (volumeMounts []k8sv1.VolumeMount) {
	for _, v := range conf.VolumeMounts {
		volumeMount := k8sv1.VolumeMount{
			Name:      v.Name,
			MountPath: v.MountPath,
			ReadOnly:  v.ReadOnly,
		}
		switch v.MountPropagation {
		case "HostToContainer":
			volumeMount.MountPropagation = &MountPropagationHostToContainer
		case "Bidirectional":
			volumeMount.MountPropagation = &MountPropagationBidirectional
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}
	return volumeMounts
}

type VolumeMount struct {
	Name             string `json:"name"`
	MountPath        string `json:"mountpath"`
	MountPropagation string `json:"mountpropagation"`
	ReadOnly         bool   `json:"read_only"`
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
// path is "/mariner.json"
func loadConfig(path string) (marinerConfig FullMarinerConfig) {
	config, _ := ioutil.ReadFile(path)
	_ = json.Unmarshal(config, &marinerConfig)
	return marinerConfig
}
