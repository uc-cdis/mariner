package mariner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/robertkrimen/otto"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for loading/processing inputs for *Tools
// for an explanation of the PreProcessContext function, see "explanation for PreProcessContext" comment in js.go

// LoadInputs passes parameter value to input.Provided for each input
// in this setting, "ValueFrom" may appear either in:
//  - tool.Root.Inputs[i].inputBinding.ValueFrom, OR
//  - tool.OriginalStep.In[i].ValueFrom
// need to handle both cases - first eval at the workflowStepInput level, then eval at the tool input level
func (tool *Tool) loadInputs() (err error) {
	sort.Sort(tool.Root.Inputs)
	fmt.Println("building step input map..")
	tool.buildStepInputMap()
	for _, in := range tool.Root.Inputs {
		fmt.Printf("loading input %v..\n", in.ID)
		err = tool.loadInput(in)
		if err != nil {
			return err
		}
	}
	return nil
}

// used in loadInput() to handle case of workflow step input valueFrom case
// FIXME - this function is busted - something to do with the pointer I'm sure
func (tool *Tool) buildStepInputMap() {
	tool.StepInputMap = make(map[string]*cwl.StepInput)
	for _, in := range tool.OriginalStep.In {
		localID := GetLastInPath(in.ID) // e.g., "file_array" instead of "#subworkflow_test.cwl/test_expr/file_array"
		tool.StepInputMap[localID] = &in
	}
}

// loadInput passes input parameter value to input.Provided
func (tool *Tool) loadInput(input *cwl.Input) (err error) {
	// transformInput() handles any valueFrom statements at the workflowStepInput level and the tool input level
	// to be clear: "workflowStepInput level" refers to this tool and its inputs as they appear as a step in a workflow
	// so that would be specified in a cwl workflow file like Workflow.cwl
	// and the "tool input level" refers to the tool and its inputs as they appear in a standalone tool specification
	// so that information would be specified in a cwl  *tool file like CommandLineTool.cwl or ExpressionTool.cwl
	if provided, err := tool.transformInput(input); err == nil {
		input.Provided = cwl.Provided{}.New(input.ID, provided)
	} else {
		fmt.Printf("error transforming input: %v\ninput: %v\n", err, input.ID)
		return err
	}

	if input.Default == nil && input.Binding == nil && input.Provided == nil {
		return fmt.Errorf("input `%s` doesn't have default field but not provided", input.ID)
	}
	if key, needed := input.Types[0].NeedRequirement(); needed {
		for _, req := range tool.Root.Requirements {
			for _, requiredtype := range req.Types {
				if requiredtype.Name == key {
					input.RequiredType = &requiredtype
					input.Requirements = tool.Root.Requirements
				}
			}
		}
	}
	return nil
}

func (tool *Tool) transformInput(input *cwl.Input) (out interface{}, err error) {
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
	localID := GetLastInPath(input.ID)
	// stepInput ValueFrom case
	if tool.StepInputMap[localID].ValueFrom == "" {
		// no processing needs to happen if the valueFrom field is empty
		fmt.Println("no value from to handle")
		var ok bool
		if out, ok = tool.Parameters[input.ID]; !ok {
			fmt.Println("error: input not found in tool's parameters")
			return nil, fmt.Errorf("input not found in tool's parameters")
		}
	} else {
		// here the valueFrom field is not empty, so we need to handle valueFrom
		valueFrom := tool.StepInputMap[localID].ValueFrom
		if strings.HasPrefix(valueFrom, "$") {
			// valueFrom is an expression that needs to be eval'd

			// get a js vm
			vm := otto.New()

			// preprocess struct/array so that fields can be accessed in vm
			// Question: how to handle non-array/struct data types?
			// --------- no preprocessing should have to happen in this case.
			self, err := PreProcessContext(tool.Parameters[input.ID])
			if err != nil {
				return nil, err
			}

			// set `self` variable in vm
			vm.Set("self", self)

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
			if out, err = EvalExpression(valueFrom, vm); err != nil {
				return nil, err
			}
		} else {
			// valueFrom is not an expression - take raw string/val as value
			out = valueFrom
		}
	}

	// fmt.Println("before creating file object:")
	// PrintJSON(out)

	// if file, need to ensure that all file attributes get populated (e.g., basename)
	if isFile(out) {
		// fmt.Println("is a file object")
		path, err := GetPath(out)
		if err != nil {
			return nil, err
		}

		// HERE -> handle the filepath prefix issue
		//
		// Mapping:
		// ---- COMMONS/<guid> -> /commons-data/by-guid/<guid>
		// ---- USER/<path> -> /user-data/<path> // not implemented yet
		// ---- <path> -> <path> // no path processing required, implies file lives in engine workspace
		if strings.HasPrefix(path, COMMONS_PREFIX) {
			GUID := strings.TrimPrefix(path, COMMONS_PREFIX)
			path = strings.Join([]string{PATH_TO_COMMONS_DATA, GUID}, "")
		}
		out = getFileObj(path)
	} else {
		fmt.Println("is not a file object")
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
			if _, f := out.(*File); f {
				// fmt.Println("context is a file")
				context, err = PreProcessContext(out)
				if err != nil {
					return nil, err
				}
			} else {
				// fmt.Println("context is not a file")
				context = out
			}
			vm.Set("self", context) // NOTE: again, will more than likely need additional context here to cover other cases
			if out, err = EvalExpression(valueFrom, vm); err != nil {
				return nil, err
			}
		} else {
			// not an expression, so no eval necessary - take raw value
			out = valueFrom
		}
	}

	// fmt.Println("Here's tranformed input:")
	// PrintJSON(out)
	return out, nil
}

// inputsToVM loads tool.Root.InputsVM with inputs context - using Input.Provided for each input
// to allow js expressions to be evaluated
func (tool *Tool) inputsToVM() (err error) {
	prefix := tool.Root.ID + "/" // need to trim this from all the input.ID's
	// fmt.Println("loading inputs to vm..")
	tool.Root.InputsVM = otto.New()
	context := make(map[string]interface{})
	var fileObj *File
	for _, input := range tool.Root.Inputs {
		/*
			fmt.Println("input:")
			PrintJSON(input)
			fmt.Println("input provided:")
			PrintJSON(input.Provided)
		*/
		inputID := strings.TrimPrefix(input.ID, prefix)
		if input.Types[0].Type == "File" {
			if input.Provided.Entry != nil {
				// no valueFrom specified in inputBinding
				if input.Provided.Entry.Location != "" {
					fileObj = getFileObj(input.Provided.Entry.Location)
				}
			} else {
				// valueFrom specified in inputBinding - resulting value stored in input.Provided.Raw
				switch input.Provided.Raw.(type) {
				case string:
					fileObj = getFileObj(input.Provided.Raw.(string))
				case *File:
					fileObj = input.Provided.Raw.(*File)
				default:
					panic("unexpected datatype representing file object in input.Provided.Raw")
				}
			}
			fileContext, err := PreProcessContext(fileObj)
			if err != nil {
				return err
			}
			context[inputID] = fileContext
		} else {
			context[inputID] = input.Provided.Raw // not sure if this will work in general - so far, so good though - need to test further
		}
	}
	// fmt.Println("Here's the context")
	// PrintJSON(context)
	tool.Root.InputsVM.Set("inputs", context)
	return nil
}
