package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/uc-cdis/mariner/wflib"
)

// affix the prefix
func addPathPrefix(in map[string]interface{}) map[string]interface{} {
	var f map[string]interface{}
	var ok bool
	var path string
	for _, v := range in {
		if f, ok = v.(map[string]interface{}); ok && f["class"].(string) == "File" {
			if path, ok = f["location"].(string); ok && path != "" {
				f["location"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
			}
			if path, ok = f["path"].(string); ok && path != "" {
				f["path"] = fmt.Sprintf("%v%v", inputPathPrefix, path)
			}
		}
	}
	return in
}

func (t *TestCase) input() (map[string]interface{}, error) {
	ext := filepath.Ext(t.Input)
	if ext != ".json" && ext != ".yaml" {
		return nil, fmt.Errorf("unexpected inputs fileext: %v", ext)
	}

	b, err := ioutil.ReadFile(t.Input)
	if err != nil {
		return nil, err
	}

	in := &map[string]interface{}{}
	switch ext {
	case ".json":
		err = json.Unmarshal(b, in)
	case ".yaml":
		err = yaml.Unmarshal(b, in)
	}
	if err != nil {
		return nil, err
	}

	input := addPathPrefix(*in)

	return input, nil
}

func (t *TestCase) tags() map[string]string {
	tags := make(map[string]string)
	tags["job"] = t.Input
	tags["tool"] = t.CWL
	tags["label"] = t.Label
	tags["id"] = string(t.ID)
	tags["doc"] = t.Doc
	tags["tags"] = strings.Join(t.Tags, ",")
	if t.ShouldFail {
		tags["should_fail"] = "true"
	} else {
		tags["should_fail"] = "false"
	}
	return tags
}

func (t *TestCase) workflow() (*wflib.WorkflowJSON, error) {
	wf, err := wflib.PackWorkflow(t.CWL)
	if err != nil {
		return nil, err
	}
	valid, grievances := wflib.ValidateWorkflow(wf)
	if !valid {
		return nil, fmt.Errorf("%v", grievances)
	}
	return wf, nil
}
