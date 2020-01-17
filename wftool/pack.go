package wftool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// error handling, in general, needs attention

// WorkflowJSON ..
type WorkflowJSON struct {
	Graph      *[]map[string]interface{} `json:"$graph"`
	CWLVersion string                    `json:"cwlVersion"`
}

// Pack is the top level function for the packing routine
func Pack(inPath string, outPath string) (err error) {
	var wf WorkflowJSON
	if wf, err = PackWorkflow(inPath); err != nil {
		return err
	}
	if err = writeJSON(wf, outPath); err != nil {
		return err
	}
	return nil
}

func writeJSON(wf WorkflowJSON, outPath string) error {
	f, err := os.Create(outPath)
	defer f.Close()
	if err != nil {
		return err
	}
	j, err := json.Marshal(wf)
	if err != nil {
		return err
	}
	f.Write(j)
	return nil
}

// PackWorkflow packs the workflow specified by cwl file with path 'path'
// i.e., packs 'path' and all child cwl files of 'path'
func PackWorkflow(path string) (WorkflowJSON, error) {

	// workflow gets packed into graph
	graph := &[]map[string]interface{}{}
	if err := PackCWLFile(path, "", graph); err != nil {
		return WorkflowJSON{}, nil
	}

	wf := WorkflowJSON{
		Graph:      graph,
		CWLVersion: "testCWLPack1.0", // fixme: populate actual value
	}
	return wf, nil
}

// PackCWL serializes a single cwl obj (e.g., commandlinetool) to json
func PackCWL(cwl []byte, id string, path string, graph *[]map[string]interface{}) (map[string]interface{}, error) {
	cwlObj := new(interface{})
	yaml.Unmarshal(cwl, cwlObj)
	j, ok := nuConvert(*cwlObj, primaryRoutine, id, false, path, graph).(map[string]interface{})
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
func PackCWLFile(path string, prevPath string, graph *[]map[string]interface{}) (err error) {
	if path, err = absPath(path, prevPath); err != nil {
		return err
	}

	cwl, err := ioutil.ReadFile(path)
	if err != nil {
		// routine should fail out here and primaryRoutine should not return any results
		fmt.Println("err 4: ", err)
		return err
	}

	// copying cwltool's pack id scheme
	// not sure if it's actually good or not
	// but for now, doing this
	id := fmt.Sprintf("#%v", filepath.Base(path))

	// 'path' here is absolute - implies prevPath is absolute
	j, err := PackCWL(cwl, id, path, graph)
	*graph = append(*graph, j)
	return nil
}

func absPath(path string, prevPath string) (string, error) {
	var err error
	var wd string
	if prevPath != "" {
		if !strings.ContainsAny(prevPath, "/") {
			prevPath = fmt.Sprintf("./%v", prevPath)
		}
		if err = os.Chdir(filepath.Dir(prevPath)); err != nil {
			fmt.Println("err 1: ", err)
			return "", err
		}
		if err = os.Chdir(filepath.Dir(path)); err != nil {
			fmt.Println("err 2: ", err)
			return "", err
		}
		if wd, err = os.Getwd(); err != nil {
			fmt.Println("err 3: ", err)
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
		fmt.Printf("error printing JSON: %v", err)
	}
	fmt.Println(string(see))
}
