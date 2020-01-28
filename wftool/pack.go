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

// Pack is the top level function for the packing routine
func Pack(inPath string, outPath string) (err error) {
	var wf *WorkflowJSON
	var wd string
	wd, _ = os.Getwd()

	defer func() {
		if r := recover(); r != nil {
			// fmt.Printf("panic: failed to pack workflow\n%v\n\n", r)
			err = fmt.Errorf("pack routine panicked")
		}
	}()

	// pack the thing
	if wf, err = PackWorkflow(inPath); err != nil {
		// fmt.Println("failed to pack workflow ", inPath)
		return err
	}

	// validate the thing
	// valid, grievances := ValidateWorkflow(wf)
	valid, _ := ValidateWorkflow(wf)
	if !valid {
		// need some more natural response here
		// fmt.Println("grievances:")
		// printJSON(grievances)
		return fmt.Errorf("workflow is not valid - see grievances")
	}

	// fmt.Println("your workflow is valid!")

	// write the thing to a file
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
		return defaultOutPath(inPath), nil
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

// same as inPath, but w ext '.json' instead of '.cwl'
func defaultOutPath(inPath string) string {
	ext := filepath.Ext(inPath)
	noExt := strings.TrimSuffix(inPath, ext)
	return fmt.Sprintf("%v.json", noExt)
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
	encoder.Encode(wf)
	return nil
}

// PackWorkflow packs the workflow specified by cwl file with path 'path'
// i.e., packs 'path' and all child cwl files of 'path'
func PackWorkflow(path string) (*WorkflowJSON, error) {

	// workflow gets packed into graph
	graph := &[]map[string]interface{}{}

	// collects versions of all cwl files in workflow
	// workflow is only valid if all versions are the same
	// i.e., this map should have exactly 1 entry in it
	versionCheck := make(map[string][]string)

	if err := PackCWLFile(path, "", graph, versionCheck); err != nil {
		return nil, err
	}

	// error if multiple cwl versions specified in workflow files
	if len(versionCheck) > 1 {
		fmt.Println("pack operation failed - incompatible versions specified")
		fmt.Println("version breakdown:")
		printJSON(versionCheck)
		return nil, fmt.Errorf("version error")
	}

	// get the one version listed
	var cwlVersion string
	for ver := range versionCheck {
		cwlVersion = ver
	}

	wf := &WorkflowJSON{
		Graph:      graph,
		CWLVersion: cwlVersion,
	}

	return wf, nil
}

// PackCWL serializes a single cwl obj (e.g., commandlinetool) to json
func PackCWL(cwl []byte, id string, path string, graph *[]map[string]interface{}, versionCheck map[string][]string) (map[string]interface{}, error) {
	cwlObj := new(interface{})
	yaml.Unmarshal(cwl, cwlObj)
	j, ok := nuConvert(*cwlObj, primaryRoutine, id, false, path, graph, versionCheck).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert %v to json", path)
	}
	return j, nil
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
func PackCWLFile(path string, prevPath string, graph *[]map[string]interface{}, versionCheck map[string][]string) (err error) {
	if path, err = absPath(path, prevPath); err != nil {
		return err
	}

	cwl, err := ioutil.ReadFile(path)
	if err != nil {
		// routine should fail out here and primaryRoutine should not return any results
		// fmt.Println("err 4: ", err)
		return err
	}

	// copying cwltool's pack id scheme
	// not sure if it's actually good or not
	// but for now, doing this
	var id string
	if prevPath == "" {
		id = "#main"
	} else {
		id = fmt.Sprintf("#%v", filepath.Base(path))
	}

	// 'path' here is absolute - implies prevPath is absolute
	j, err := PackCWL(cwl, id, path, graph, versionCheck)
	*graph = append(*graph, j)
	return nil
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

// PrintJSON pretty prints a struct as JSON
func printJSON(i interface{}) {
	var see []byte
	var err error
	see, err = json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v\n", err)
	}
	fmt.Println(string(see))
}
