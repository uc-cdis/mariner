package mariner

import (
	"encoding/json"
	"io/ioutil"
)

// this file contains various config vars, consts, type definitions

type FullMarinerConfig struct {
	Config MarinerConfig `json:"mariner"`
}

type MarinerConfig struct {
	Containers Containers `json:"containers"`
	Jobs       Jobs       `json:"jobs"`
	Secrets    Secrets    `json:"secrets"`
}

type Containers struct {
	Engine          Container `json:"engine"`
	S3sidecar       Container `json:"s3sidecar"`
	CommandLineTool Container `json:"commandlinetool"`
}

type Container struct {
	Name            string          `json:"name"`
	Image           string          `json:"image"`
	PullPolicy      string          `json:"pull_policy"`
	Command         []string        `json:"command"`
	VolumeMounts    []VolumeMount   `json:"volume_mounts"`
	SecurityContext SecurityContext `json:"securitycontext"`
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
	Labels         Labels `json:"labels"`
	ServiceAccount string `json:"serviceaccount"`
	RestartPolicy  string `json:"restart_policy"`
}

type Labels struct {
	App string `json:"app"`
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
