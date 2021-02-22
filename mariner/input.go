package mariner

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for loading/processing inputs for *Tools
// for an explanation of the PreProcessContext function, see "explanation for PreProcessContext" comment in js.go

// LoadInputs passes parameter value to input.Provided for each input
// in this setting, "ValueFrom" may appear either in:
//  - tool.Task.Root.Inputs[i].inputBinding.ValueFrom, OR
//  - tool.OriginalStep.In[i].ValueFrom
// need to handle both cases - first eval at the workflowStepInput level, then eval at the tool input level
// if err and input is not optional, it is a fatal error and the run should fail out
func (engine *K8sEngine) loadInputs(tool *Tool) (err error) {
	tool.Task.infof("begin load inputs")
	sort.Sort(tool.Task.Root.Inputs)
	tool.buildStepInputMap()
	for _, in := range tool.Task.Root.Inputs {
		if err = engine.loadInput(tool, in); err != nil {
			return tool.Task.errorf("failed to load input: %v", err)
		}
		// map parameter to value for log
		if in.Provided != nil {
			tool.Task.Log.Input[in.ID] = in.Provided.Raw
		} else {
			// implies an unused input parameter
			// e.g., an optional input with no value or default provided
			tool.Task.Log.Input[in.ID] = nil
		}
	}
	tool.Task.infof("end load inputs")
	return nil
}

// used in loadInput() to handle case of workflow step input valueFrom case
func (tool *Tool) buildStepInputMap() {
	// if this tool is not a child task of some parent workflow
	// i.e., if this whole "workflow" consists of just a single tool
	if tool.Task.OriginalStep == nil {
		tool.Task.infof("tool has no parent workflow")
		return
	}

	tool.Task.infof("begin build step input map")
	tool.StepInputMap = make(map[string]*cwl.StepInput)
	// serious "gotcha": https://medium.com/@betable/3-go-gotchas-590b8c014e0a
	/*
		"Go uses a copy of the value instead of the value itself within a range clause.
		So when we take the pointer of value, we’re actually taking the pointer of a copy
		of the value. This copy gets reused throughout the range clause [...]"
	*/
	for j := range tool.Task.OriginalStep.In {
		localID := lastInPath(tool.Task.OriginalStep.In[j].ID)
		tool.StepInputMap[localID] = &tool.Task.OriginalStep.In[j]
	}
	tool.Task.infof("end build step input map")
}

// loadInput passes input parameter value to input.Provided
func (engine *K8sEngine) loadInput(tool *Tool, input *cwl.Input) (err error) {
	tool.Task.infof("begin load input: %v", input.ID)

	// transformInput() handles any valueFrom statements at the workflowStepInput level and the tool input level
	// to be clear: "workflowStepInput level" refers to this tool and its inputs as they appear as a step in a workflow
	// so that would be specified in a cwl workflow file like Workflow.cwl
	// and the "tool input level" refers to the tool and its inputs as they appear in a standalone tool specification
	// so that information would be specified in a cwl tool file like CommandLineTool.cwl or ExpressionTool.cwl
	required := true
	if provided, err := engine.transformInput(tool, input); err == nil {
		if provided == nil {
			// optional input with no value or default provided
			// this is an unused input parameter
			// and so does not show up on the command line
			// so here we set the binding to nil to signal to mariner later on
			// to not look at this input when building the tool command
			required = false
			input.Binding = nil
		}
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	} else {
		return tool.Task.errorf("failed to transform input: %v; error: %v", input.ID, err)
	}

	if required && input.Default == nil && input.Binding == nil && input.Provided == nil {
		return tool.Task.errorf("required input %s value not provided and no default specified", input.ID)
	}
	if key, needed := input.Types[0].NeedRequirement(); needed {
		for _, req := range tool.Task.Root.Requirements {
			for _, requiredtype := range req.Types {
				if requiredtype.Name == key {
					input.RequiredType = &requiredtype
					input.Requirements = tool.Task.Root.Requirements
				}
			}
		}
	}
	tool.Task.infof("end load input: %v", input.ID)
	return nil
}

// wrapper around processFile() - collects path of input file and all secondary files
func (tool *Tool) processFile(f interface{}) (*File, error) {
	obj, err := processFile(f)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(obj.Path, pathToCommonsData) {
		tool.S3Input.Paths = append(tool.S3Input.Paths, obj.Path)
	}

	// note: I don't think any input to this process will have secondary files loaded
	// into this field at this point in the process
	for _, sf := range obj.SecondaryFiles {
		if !strings.HasPrefix(sf.Path, pathToCommonsData) {
			tool.S3Input.Paths = append(tool.S3Input.Paths, sf.Path)
		}
	}
	return obj, nil
}

// called in transformInput() routine
// handles path prefix issue
func processFile(f interface{}) (*File, error) {

	// if it's already of type File or *File, it requires no processing
	if obj, ok := f.(File); ok {
		// "reset" secondaryFiles field to nil
		obj.SecondaryFiles = nil
		return &obj, nil
	}
	if p, ok := f.(*File); ok {
		// process a copy of the original file
		// reset secondaryFiles field to nil
		fileObj := *p
		fileObj.SecondaryFiles = nil
		return &fileObj, nil
	}

	path, err := filePath(f)
	if err != nil {
		return nil, err
	}

	// handle the filepath prefix issue
	//
	// Mapping:
	// ---- COMMONS/<guid> -> /commons-data/by-guid/<guid>
	// ---- USER/<path> -> /user-data/<path> // not implemented yet
	// ---- <path> -> <path> // no path processing required, implies file lives in engine workspace
	switch {
	case strings.HasPrefix(path, commonsPrefix):
		/*
			~ Path represenations/handling for commons-data ~

			inputs.json: 				"COMMONS/<guid>"
			mariner: "/commons-data/data/by-guid/<guid>"

			gen3-fuse mounts those objects whose GUIDs appear in the manifest
			gen3-fuse mounts to /commons-data/data/
			and mounts the directories "by-guid/", "by-filepath/", and "by-filename/"

			commons files can be accessed via the full path "/commons-data/data/by-guid/<guid>"

			so the mapping that happens in this path processing step is this:
			"COMMONS/<guid>" -> "/commons-data/data/by-guid/<guid>"
		*/
		GUID := strings.TrimPrefix(path, commonsPrefix)
		path = strings.Join([]string{pathToCommonsData, GUID}, "")
	case strings.HasPrefix(path, userPrefix):
		/*
			~ Path representations/handling for user-data ~

			s3: 			   "/userID/path/to/file"
			inputs.json: 	      "USER/path/to/file"
			mariner: 		"/engine-workspace/path/to/file"

			user-data bucket gets mounted at the 'userID' prefix to dir /engine-workspace/

			so the mapping that happens in this path processing step is this:
			"USER/path/to/file" -> "/engine-workspace/path/to/file"
		*/
		trimmedPath := strings.TrimPrefix(path, userPrefix)
		path = strings.Join([]string{"/", engineWorkspaceVolumeName, "/", trimmedPath}, "")
	case strings.HasPrefix(path, conformancePrefix):
		trimmedPath := strings.TrimPrefix(path, conformancePrefix)
		path = strings.Join([]string{"/", conformanceVolumeName, "/", trimmedPath}, "")
	}
	return fileObject(path), nil
}

// called in transformInput() routine
func (tool *Tool) processFileList(l interface{}) ([]*File, error) {
	if reflect.TypeOf(l).Kind() != reflect.Array {
		return nil, fmt.Errorf("not an array")
	}

	var err error
	var f *File
	var i interface{}
	out := []*File{}
	s := reflect.ValueOf(l)
	for j := 0; j < s.Len(); j++ {
		i = s.Index(j)
		if !isFile(i) {
			return nil, fmt.Errorf("nonFile object found in file array: %v", i)
		}
		if f, err = tool.processFile(i); err != nil {
			return nil, fmt.Errorf("failed to process file %v", i)
		}
		out = append(out, f)
	}
	return out, nil
}

// transformInput parses all input in a workflow from the engine's tool.
func (engine *K8sEngine) transformInput(tool *Tool, input *cwl.Input) (out interface{}, err error) {
	tool.Task.infof("begin transform input: %v", input.ID)
	localID := lastInPath(input.ID)
	if tool.StepInputMap[localID] != nil {
		if tool.StepInputMap[localID].ValueFrom != "" {
			valueFrom := tool.StepInputMap[localID].ValueFrom
			if strings.HasPrefix(valueFrom, "$") {
				vm := tool.JSVM.Copy()
				self, err := tool.loadInputValue(input)
				if err != nil {
					return nil, tool.Task.errorf("failed to load value: %v", err)
				}
				self, err = preProcessContext(self)
				if err != nil {
					return nil, tool.Task.errorf("failed to preprocess context: %v", err)
				}
				if err = vm.Set("self", self); err != nil {
					return nil, tool.Task.errorf("failed to set 'self' value in js vm: %v", err)
				}
				if out, err = evalExpression(valueFrom, vm); err != nil {
					return nil, tool.Task.errorf("failed to eval js expression: %v; error: %v", valueFrom, err)
				}
			} else {
				out = valueFrom
			}
		}
	}

	if out == nil {
		out, err = tool.loadInputValue(input)
		if err != nil {
			return nil, tool.Task.errorf("failed to load input value: %v", err)
		}
		if out == nil {
			tool.Task.infof("optional input with no value or default provided - skipping: %v", input.ID)
			return nil, nil
		}
	}

	switch {
	case isFile(out):
		if out, err = tool.processFile(out); err != nil {
			return nil, tool.Task.errorf("failed to process file: %v; error: %v", out, err)
		}
	case isArrayOfFile(out):
		if out, err = tool.processFileList(out); err != nil {
			return nil, tool.Task.errorf("failed to process file list: %v; error: %v", out, err)
		}
	default:
		tool.Task.infof("input is not a file object: %v", input.ID)
	}

	if len(input.SecondaryFiles) > 0 {
		var fileArray []*File
		switch {
		case isFile(out):
			fileArray = []*File{out.(*File)}
		case isArrayOfFile(out):
			fileArray = out.([]*File)
		default:
			return nil, tool.Task.errorf("invalid input: secondary files specified for a non-file input.")
		}
		for _, entry := range input.SecondaryFiles {
			val := entry.Entry
			if strings.HasPrefix(val, "$") {
				vm := tool.JSVM.Copy()
				for _, fileObj := range fileArray {
					self, err := preProcessContext(fileObj)
					if err != nil {
						return nil, tool.Task.errorf("%v", err)
					}
					vm.Set("self", self)
					jsResult, err := evalExpression(val, vm)
					if err != nil {
						return nil, tool.Task.errorf("%v", err)
					}
					sFilePath, ok := jsResult.(string)
					if !ok {
						return nil, tool.Task.errorf("secondaryFile expression did not return string")
					}
					if exist, _ := engine.fileExists(sFilePath); !exist {
						return nil, tool.Task.errorf("secondary file doesn't exist")
					}
					sFileObj := fileObject(sFilePath)
					fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFileObj)
				}
			} else {
				suffix, carats := trimLeading(val, "^")
				for _, fileObj := range fileArray {
					engine.loadSFilesFromPattern(tool, fileObj, suffix, carats)
				}
			}
		}
		for _, fileObj := range fileArray {
			for _, sf := range fileObj.SecondaryFiles {
				if !strings.HasPrefix(sf.Location, pathToCommonsData) {
					tool.S3Input.Paths = append(tool.S3Input.Paths, sf.Location)
				}
			}
		}
	}

	if input.Binding != nil && input.Binding.ValueFrom != nil {
		valueFrom := input.Binding.ValueFrom.String
		if strings.HasPrefix(valueFrom, "$") {
			vm := tool.JSVM.Copy()
			var context interface{}
			switch out.(type) {
			case *File, []*File:
				context, err = preProcessContext(out)
				if err != nil {
					return nil, tool.Task.errorf("failed to preprocess context: %v", err)
				}
			default:
				context = out
			}
			vm.Set("self", context)
			if out, err = evalExpression(valueFrom, vm); err != nil {
				return nil, tool.Task.errorf("failed to eval expression: %v; error: %v", valueFrom, err)
			}
		} else {
			out = valueFrom
		}
	}
	tool.Task.infof("end transform input: %v", input.ID)
	return out, nil
}

/*
loadInputValue logic:
1. take value from params
2. if not given in params:
	i) take default value
	ii) if no default provided:
		a) if optional param, return nil, nil
		b) if required param, return nil, err (fatal error)
*/

// handles all cases of input params
// i.e., handles all optional/null/default param/value logic
func (tool *Tool) loadInputValue(input *cwl.Input) (out interface{}, err error) {
	tool.Task.infof("begin load input value for input: %v", input.ID)
	var required, ok bool
	// take value from given param value set
	out, ok = tool.Task.Parameters[input.ID]
	// if no value exists in the provided parameter map
	if !ok || out == nil {
		// check if default value is provided
		if input.Default == nil {
			// implies no default value provided

			// determine if this param is required or optional
			required = true
			for _, t := range input.Types {
				if t.Type == CWLNullType {
					required = false
				}
			}

			// return error if this is a required param
			if required {
				return nil, tool.Task.errorf("missing value for required input param %v", input.ID)
			}
		} else {
			// implies a default value is provided
			// in this case you return the default value
			out = input.Default.Self
		}
	}
	tool.Task.infof("end load input value for input: %v", input.ID)
	return out, nil
}

// general (very important) note
// the Task.Root object - that "Root" object
// is shared among all instances of a run of that root object
// for example, say two steps of a workflow run the same tool
// -> there's one "root" object which represents the tool
// the tool gets run twice, let's say concurrently
// so there are TWO TASKS WHICH POINT TO THE SAME ROOT
// that is to say
// it's SAFE TO READ from the root
// but it's NOT SAFE TO WRITE to the root, under any circumstances
// lest ye enjoy endless pointer / recursion / concurrency debugging

// inputsToVM loads tool.InputsVM with inputs context - using Input.Provided for each input
// to allow js expressions to be evaluated
func (tool *Tool) inputsToVM() (err error) {
	tool.Task.infof("begin load inputs to js vm")
	prefix := tool.Task.Root.ID + "/" // need to trim this from all the input.ID's
	tool.InputsVM = tool.JSVM.Copy()
	context := make(map[string]interface{})
	var f interface{}
	for _, input := range tool.Task.Root.Inputs {
		inputID := strings.TrimPrefix(input.ID, prefix)

		// fixme: handle array of files
		// note: this code block is extraordinarily janky and needs to be refactored
		// error here.
		switch {
		case input.Provided == nil:
			context[inputID] = nil
		case input.Types[0].Type == CWLFileType:
			if input.Provided.Entry != nil {
				// no valueFrom specified in inputBinding
				if input.Provided.Entry.Location != "" {
					f = fileObject(input.Provided.Entry.Location)
				}
			} else {
				// valueFrom specified in inputBinding - resulting value stored in input.Provided.Raw
				switch input.Provided.Raw.(type) {
				case string:
					f = fileObject(input.Provided.Raw.(string))
				case *File, []*File:
					f = input.Provided.Raw
				default:
					return tool.Task.errorf("unexpected datatype representing file object in input.Provided.Raw")
				}
			}
			fileContext, err := preProcessContext(f)
			if err != nil {
				return tool.Task.errorf("failed to preprocess file context: %v; error: %v", f, err)
			}
			context[inputID] = fileContext
		default:
			context[inputID] = input.Provided.Raw // not sure if this will work in general - so far, so good though - need to test further
		}
	}
	if err = tool.InputsVM.Set("inputs", context); err != nil {
		return tool.Task.errorf("failed to set inputs context in js vm: %v", err)
	}
	tool.Task.infof("end load inputs to js vm")
	return nil
}
