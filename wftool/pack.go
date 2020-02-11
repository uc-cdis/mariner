package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// error handling and validation, in general, need attention
// code needs better organization

// Packer ..
type Packer struct {
	Graph        *[]map[string]interface{}
	FilesPacked  map[string]string // {path: id}
	VersionCheck map[string][]string
}

// PackWorkflow ..
func PackWorkflow(inPath string) (wf *WorkflowJSON, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pack routine panicked")
		}
	}()

	packer := &Packer{
		Graph:        &[]map[string]interface{}{},
		FilesPacked:  make(map[string]string),
		VersionCheck: make(map[string][]string),
	}

	if wf, err = packer.PackWorkflow(inPath); err != nil {
		return nil, err
	}
	return wf, nil
}

// Pack is the top level function for the packing routine
func Pack(inPath string, outPath string) (err error) {
	var wd string
	wd, _ = os.Getwd()

	wf, err := PackWorkflow(inPath)
	if err != nil {
		return err
	}

	valid, grievances := ValidateWorkflow(wf)
	if !valid {
		fmt.Println("grievances:")
		printJSON(grievances)
		return fmt.Errorf("workflow is not valid - see grievances")
	}

	if outPath, err = resolveOutPath(inPath, outPath, wd); err != nil {
		return err
	}
	if err = writeJSON(wf, outPath); err != nil {
		return err
	}
	return err
}

func resolveOutPath(inPath string, outPath string, wd string) (string, error) {

	// no outPath specified
	if outPath == "" {
		return defaultOutPath(inPath, wd), nil
	}

	// full outPath specified
	if filepath.IsAbs(outPath) {
		return outPath, nil
	}

	// outPath specified relative to wd
	var err error
	if outPath, err = absPath(outPath, wd); err != nil {
		return "", err
	}
	return outPath, nil
}

func defaultOutPath(inPath, wd string) string {
	ext := filepath.Ext(inPath)
	noExt := strings.TrimSuffix(filepath.Base(inPath), ext)
	return filepath.Join(wd, fmt.Sprintf("%v_packed.json", noExt))
}

func writeJSON(wf *WorkflowJSON, outPath string) error {
	f, err := os.Create(outPath)
	defer f.Close()
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(wf); err != nil {
		return err
	}
	return nil
}

// PackWorkflow packs the workflow specified by cwl file with path 'path'
// i.e., packs 'path' and all child cwl files of 'path'
func (p *Packer) PackWorkflow(path string) (*WorkflowJSON, error) {
	// workflow gets packed into graph

	if _, err := p.PackCWLFile(path, ""); err != nil {
		return nil, err
	}

	// error if multiple cwl versions specified in workflow files
	if len(p.VersionCheck) > 1 {
		fmt.Println("pack operation failed - incompatible versions specified")
		fmt.Println("version breakdown:")
		printJSON(p.VersionCheck)
		return nil, fmt.Errorf("version error")
	}

	// get the one version listed
	var cwlVersion string
	for ver := range p.VersionCheck {
		cwlVersion = ver
	}

	wf := &WorkflowJSON{
		Graph:      p.Graph,
		CWLVersion: cwlVersion,
	}
	return wf, nil
}

// dev'ing
func resolveID(i interface{}, defaultID string) (string, error) {
	cwlObj, ok := i.(map[interface{}]interface{})
	if !ok {
		return "", fmt.Errorf("invalid document structure")
	}
	if givenID, ok := cwlObj["id"]; ok && defaultID != mainID {
		return fmt.Sprintf("#%v", givenID.(string)), nil
	}
	return defaultID, nil
}

// PackCWL serializes a single cwl obj (e.g., a commandlinetool) to json
func (p *Packer) PackCWL(cwl []byte, defaultID string, path string) (map[string]interface{}, string, error) {
	cwlObj := new(interface{})
	yaml.Unmarshal(cwl, cwlObj)
	id, err := resolveID(*cwlObj, defaultID)
	if err != nil {
		return nil, "", err
	}
	i, err := p.convert(*cwlObj, primaryRoutine, id, false, path)
	if err != nil {
		return nil, "", err
	}
	j, ok := i.(map[string]interface{})
	if !ok {
		return nil, "", fmt.Errorf("failed to convert %v to json", path)
	}
	return j, id, nil
}

// PackCWLFile ..
// 'path' is relative to prevPath
// except in the case where prevPath is "", and path is absolute
// which is the first call to packCWLFile
//
// at first call
// first try absolute path
// if err, try relative path - path relative to working dir
// if err, fail out
//
// always only handle absolute paths - keep things simple
// assume prevPath is absolute
// and path is relative to prevPath
// construct absolute path of `path`
//
// so:
// 'path' is relative to 'prevPath'
// 'prevPath' is absolute
// 1. construct abs(path)
// 2. ..
func (p *Packer) PackCWLFile(path string, prevPath string) (string, error) {
	var err error
	if filepath.Ext(path) != ".cwl" {
		return "", fmt.Errorf("input %v is not a cwl file", path)
	}
	if path, err = absPath(path, prevPath); err != nil {
		return "", err
	}

	// if this file has already been packed
	// skip it, and return the URI of the already packed object
	if packedID, ok := p.FilesPacked[path]; ok {
		return packedID, nil
	}

	cwl, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	var defaultID string
	if prevPath == "" {
		defaultID = mainID
	} else {
		/*
			if a user for whatever reason has two different files
			with the same base name but in different directories
			this is gonna be a problem
			the two different files will be packed with the same ID

			fixme
		*/
		defaultID = fmt.Sprintf("#%v", filepath.Base(path))
	}

	// 'path' here is absolute - implies prevPath is absolute
	j, id, err := p.PackCWL(cwl, defaultID, path)
	if err != nil {
		fmt.Println("error from PackCWL")
		return "", err
	}

	*p.Graph = append(*p.Graph, j)
	p.FilesPacked[path] = id
	return id, nil
}

// this feels like a sin
// but not sure offhand how to otherwise handle resolving paths
func absPath(path string, refPath string) (string, error) {
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	var wd string
	if refPath != "" {
		refInfo, err := os.Stat(refPath)
		if err != nil {
			return "", err
		}
		if refInfo.IsDir() {
			refPath = fmt.Sprintf("%v/", refPath)
		}
		if err = os.Chdir(filepath.Dir(refPath)); err != nil {
			return "", err
		}
		if err = os.Chdir(filepath.Dir(path)); err != nil {
			return "", err
		}
		if wd, err = os.Getwd(); err != nil {
			return "", err
		}
		path = fmt.Sprintf("%v/%v", wd, filepath.Base(path))
	}
	return path, nil
}

// printJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	see, err := json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
