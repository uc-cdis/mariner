package mariner

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/robertkrimen/otto"

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
func (tool *Tool) loadInputs() (err error) {
	tool.Task.infof("begin load inputs")
	sort.Sort(tool.Task.Root.Inputs)
	tool.buildStepInputMap()
	for _, in := range tool.Task.Root.Inputs {
		if err = tool.loadInput(in); err != nil {
			return tool.Task.errorf("failed to load input: %v", err)
		}
		// map parameter to value for log
		tool.Task.Log.Input[in.ID] = in.Provided.Raw
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
	tool.StepInputMap = &GoStringToStepInput{
		Map: make(map[string]*cwl.StepInput),
	}
	for _, in := range tool.Task.OriginalStep.In {
		localID := lastInPath(in.ID) // e.g., "file_array" instead of "#subworkflow_test.cwl/test_expr/file_array"
		tool.StepInputMap.update(localID, &in)
	}
	tool.Task.infof("end build step input map")
}

// loadInput passes input parameter value to input.Provided
func (tool *Tool) loadInput(input *cwl.Input) (err error) {
	tool.Task.infof("begin load input: %v", input.ID)
	// transformInput() handles any valueFrom statements at the workflowStepInput level and the tool input level
	// to be clear: "workflowStepInput level" refers to this tool and its inputs as they appear as a step in a workflow
	// so that would be specified in a cwl workflow file like Workflow.cwl
	// and the "tool input level" refers to the tool and its inputs as they appear in a standalone tool specification
	// so that information would be specified in a cwl tool file like CommandLineTool.cwl or ExpressionTool.cwl
	if provided, err := tool.transformInput(input); err == nil {
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	} else {
		return tool.Task.errorf("failed to transform input: %v; error: %v", input.ID, err)
	}

	if input.Default == nil && input.Binding == nil && input.Provided == nil {
		return tool.Task.errorf("input %s not provided and no default specified", input.ID)
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

// called in transformInput() routine
// handles path prefix issue
func processFile(f interface{}) (*File, error) {
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
func processFileList(l interface{}) ([]*File, error) {
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
		if f, err = processFile(i); err != nil {
			return nil, fmt.Errorf("failed to process file %v", i)
		}
		out = append(out, f)
	}
	return out, nil
}

// if err and input is not optional, it is a fatal error and the run should fail out
func (tool *Tool) transformInput(input *cwl.Input) (out interface{}, err error) {
	tool.Task.infof("begin transform input: %v", input.ID)
	/*
		NOTE: presently only context loaded into js vm's here is `self`
		Will certainly need to add more context to handle all cases
		Definitely, definitely need a generalized method for loading appropriate context at appropriate places
		In particular, the `inputs` context is probably going to be needed most commonly

		OTHERNOTE: `self` (in js vm) takes on different values in different places, according to cwl docs
		see: https://www.commonwl.org/v1.0/Workflow.html#Parameter_references
		---
		Steps:
		1. handle ValueFrom case at stepInput level
		 - if no ValueFrom specified, assign parameter value to `out` to processed in next step
		2. handle ValueFrom case at toolInput level
		 - initial value is `out` from step 1
	*/
	localID := lastInPath(input.ID)

	// stepInput ValueFrom case
	if len(tool.StepInputMap.Map) > 0 {
		// no processing needs to happen if the valueFrom field is empty
		if tool.StepInputMap.read(localID).ValueFrom != "" {
			// here the valueFrom field is not empty, so we need to handle valueFrom
			valueFrom := tool.StepInputMap.read(localID).ValueFrom
			if strings.HasPrefix(valueFrom, "$") {
				// valueFrom is an expression that needs to be eval'd

				// get a js vm
				vm := otto.New()

				// preprocess struct/array so that fields can be accessed in vm
				// Question: how to handle non-array/struct data types?
				// --------- no preprocessing should have to happen in this case.
				self, err := tool.loadInputValue(input)
				if err != nil {
					return nil, tool.Task.errorf("failed to load value: %v", err)
				}
				self, err = preProcessContext(self)
				if err != nil {
					return nil, tool.Task.errorf("failed to preprocess context: %v", err)
				}

				// set `self` variable in vm
				if err = vm.Set("self", self); err != nil {
					return nil, tool.Task.errorf("failed to set 'self' value in js vm: %v", err)
				}

				/*
					// Troubleshooting js
					// note: when accessing object fields using keys must use otto.Run("obj.key"), NOT otto.Get("obj.key")

					fmt.Println("self in js:")
					jsSelf, err := vm.Get("self")
					jsSelfVal, err := jsSelf.Export()
					PrintJSON(jsSelfVal)

					fmt.Println("Expression:")
					PrintJSON(valueFrom)

					fmt.Println("Object.keys(self)")
					keys, err := vm.Run("Object.keys(self)")
					if err != nil {
						fmt.Printf("Error evaluating Object.keys(self): %v\n", err)
					}
					keysVal, err := keys.Export()
					PrintJSON(keysVal)
				*/

				//  eval the expression in the vm, capture result in `out`
				if out, err = evalExpression(valueFrom, vm); err != nil {
					return nil, tool.Task.errorf("failed to eval js expression: %v; error: %v", valueFrom, err)
				}
			} else {
				// valueFrom is not an expression - take raw string/val as value
				out = valueFrom
			}
		}
	}

	// if this tool is not a step of a parent workflow
	// OR
	// if this tool is a step of a parent workflow but the valueFrom is empty
	if out == nil {
		out, err = tool.loadInputValue(input)
		if err != nil {
			return nil, tool.Task.errorf("failed to load input value: %v", err)
		}
	}

	// fmt.Println("before creating file object:")
	// PrintJSON(out)

	// if file, need to ensure that all file attributes get populated (e.g., basename)
	/*
		fixme: handle array of files
		Q: what about directories (?)

		do this:

		switch statement:
		case file
		case []file

		note: check types in the param type list?
		vs. checking types of actual values
	*/
	switch {
	case isFile(out):
		if out, err = processFile(out); err != nil {
			return nil, tool.Task.errorf("failed to process file: %v; error: %v", out, err)
		}
	case isArrayOfFile(out):
		if out, err = processFileList(out); err != nil {
			return nil, tool.Task.errorf("failed to process file list: %v; error: %v", out, err)
		}
	default:
		// fmt.Println("is not a file object")
	}

	// fmt.Println("after creating file object:")
	// PrintJSON(out)

	// at this point, variable `out` is the transformed input thus far (even if no transformation actually occured)
	// so `out` will be what we work with in this next block as an initial value
	// tool inputBinding ValueFrom case
	if input.Binding != nil && input.Binding.ValueFrom != nil {
		valueFrom := input.Binding.ValueFrom.String
		// fmt.Println("here is valueFrom:")
		// fmt.Println(valueFrom)
		if strings.HasPrefix(valueFrom, "$") {
			vm := otto.New()
			var context interface{}
			// fixme: handle array of files
			switch out.(type) {
			case *File, []*File:
				// fmt.Println("context is a file or array of files")
				context, err = preProcessContext(out)
				if err != nil {
					return nil, tool.Task.errorf("failed to preprocess context: %v", err)
				}
			default:
				// fmt.Println("context is not a file")
				context = out
			}

			vm.Set("self", context) // NOTE: again, will more than likely need additional context here to cover other cases
			if out, err = evalExpression(valueFrom, vm); err != nil {
				return nil, tool.Task.errorf("failed to eval expression: %v; error: %v", valueFrom, err)
			}
		} else {
			// not an expression, so no eval necessary - take raw value
			out = valueFrom
		}
	}

	// fmt.Println("Here's tranformed input:")
	// PrintJSON(out)
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
	// 1. take value from given param value set
	out, ok = tool.Task.Parameters[input.ID]
	if !ok || out == nil {
		// 2. take default value
		if out = input.Default.Self; out == nil {
			// so there's no value provided in the params
			// AND there's no default value provided

			// 3. determine if this param is required or optional
			required = true
			for _, t := range input.Types {
				if t.Type == CWLNullType {
					required = false
				}
			}

			// 4. return error if this is a required param
			if required {
				return nil, tool.Task.errorf("missing value for required input param %v", input.ID)
			}
		}
	}
	tool.Task.infof("end load input value for input: %v", input.ID)
	return out, nil
}

// inputsToVM loads tool.Task.Root.InputsVM with inputs context - using Input.Provided for each input
// to allow js expressions to be evaluated
func (tool *Tool) inputsToVM() (err error) {
	tool.Task.infof("begin load inputs to js vm")
	prefix := tool.Task.Root.ID + "/" // need to trim this from all the input.ID's
	tool.Task.Root.InputsVM = otto.New()
	context := make(map[string]interface{})
	var f interface{}
	for _, input := range tool.Task.Root.Inputs {
		/*
			fmt.Println("input:")
			PrintJSON(input)
			fmt.Println("input provided:")
			PrintJSON(input.Provided)
		*/
		inputID := strings.TrimPrefix(input.ID, prefix)

		// fixme: handle array of files
		// note: this code block is extraordinarily janky and needs to be refactored
		// error here.
		if input.Types[0].Type == CWLFileType {
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
		} else {
			context[inputID] = input.Provided.Raw // not sure if this will work in general - so far, so good though - need to test further
		}
	}
	if err = tool.Task.Root.InputsVM.Set("inputs", context); err != nil {
		return tool.Task.errorf("failed to set inputs context in js vm: %v", err)
	}
	tool.Task.infof("end load inputs to js vm")
	return nil
}
