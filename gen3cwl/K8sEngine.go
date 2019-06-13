package gen3cwl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/robertkrimen/otto"
	cwl "github.com/uc-cdis/cwl.go"
)

// Engine ...
type Engine interface {
	DispatchTask(jobID string, task *Task) error
}

// K8sEngine uses k8s Job API to run workflows
// currently handles all *Tools - including expression tools - should these functionalities be decoupled?
type K8sEngine struct {
	TaskSequence    []string            // for testing purposes
	Commands        map[string][]string // also for testing purposes
	UnfinishedProcs map[string]*Process // engine's stack of CLT's that are running (task.Root.ID, Process) pairs
	FinishedProcs   map[string]*Process // engine's stack of completed processes (task.Root.ID, Process) pairs
	// JobsClient      JobInterface
}

// Process represents a leaf in the graph of a workflow
// i.e., a Process is either a CommandLineTool or an ExpressionTool
// If Process is a CommandLineTool, then it gets run as a k8s job in its own container
// When a k8s job gets created, a Process struct gets pushed onto the k8s engine's stack of UnfinishedProcs
// the k8s engine continously iterates through the stack of running procs, retrieving job status from k8s api
// as soon as a job is complete, the Process struct gets popped from the stack
// and a function is called to collect the output from that completed process
//
// presently ExpressionTools run in a js vm "in the workflow engine", so they don't get dispatched as k8s jobs
type Process struct {
	JobName string // if a k8s job (i.e., if a CommandLineTool)
	JobID   string // if a k8s job (i.e., if a CommandLineTool)
	Tool    *Tool
	Task    *Task
}

// Tool represents a workflow *Tool - i.e., a CommandLineTool or an ExpressionTool
type Tool struct {
	Outdir           string // Given by context
	Root             *cwl.Root
	Parameters       cwl.Parameters
	Command          *exec.Cmd
	OriginalStep     cwl.Step
	StepInputMap     map[string]*cwl.StepInput // see: transformInput()
	ExpressionResult map[string]interface{}    // storing the result of an expression tool here for now - maybe there's a better way to do this
}

// File represents a CWL file object
// NOTE: the json representation of field names is what gets loaded into js vm
// ----- see PreProcessContext() and accompanying note of explanation.
// ----- these json aliases are the fieldnames defined by cwl for cwl File objects
//
// see: see: https://www.commonwl.org/v1.0/Workflow.html#File
type File struct {
	Class          string  `json:"class"`          // always "File"
	Location       string  `json:"location"`       // path to file (same as `path`)
	Path           string  `json:"path"`           // path to file
	Basename       string  `json:"basename"`       // last element of location path
	NameRoot       string  `json:"nameroot"`       // basename without file extension
	NameExt        string  `json:"nameext"`        // file extension of basename
	Contents       string  `json:"contents"`       // first 64 KiB of file as a string, if loadContents is true
	SecondaryFiles []*File `json:"secondaryFiles"` // array of secondaryFiles
}

// instantiates a new file object given a filepath
// returns pointer to the new File object
// presently loading both `path` and `location` for sake of loading all potentially needed context to js vm
// right now they hold the exact same path
func getFileObj(path string) (fileObj *File) {
	base, root, ext := getFileFields(path)
	fileObj = &File{
		Class:    "File",
		Location: path,
		Path:     path,
		Basename: base,
		NameRoot: root,
		NameExt:  ext,
	}
	return fileObj
}

// pedantic splitting regarding leading periods in the basename
// see: https://www.commonwl.org/v1.0/Workflow.html#File
// the description of nameroot and nameext
func getFileFields(path string) (base string, root string, ext string) {
	base = GetLastInPath(path)
	baseNoLeadingPeriods, nPeriods := trimLeading(base, ".")
	tmp := strings.Split(baseNoLeadingPeriods, ".")
	if len(tmp) == 1 {
		// no file extension
		root = tmp[0]
		ext = ""
	} else {
		root = strings.Join(tmp[:len(tmp)-1], ".")
		ext = "." + tmp[len(tmp)-1]
	}
	// add back any leading periods that were trimmed from base
	root = strings.Repeat(".", nPeriods) + root
	return base, root, ext
}

// PrintJSON pretty prints a struct as json
func PrintJSON(i interface{}) {
	var see []byte
	var err error
	see, err = json.MarshalIndent(i, "", "   ")
	if err != nil {
		fmt.Printf("error printing JSON: %v", err)
	}
	fmt.Println(string(see))
}

// GetTool returns a Tool interface
// The Tool represents a workflow *Tool and so is either a CommandLineTool or an ExpressionTool
// tool looks like mostly a subset of task..
// code needs to be polished/organized/refactored once the engine is actually running properly
func (task *Task) getTool() *Tool {
	tool := &Tool{
		Root:         task.Root,
		Parameters:   task.Parameters,
		OriginalStep: task.originalStep,
	}
	return tool
}

// LoadInputs passes parameter value to input.Provided for each input
// TODO: Handle the "ValueFrom" case
// see: https://www.commonwl.org/user_guide/13-expressions/index.html
// in this setting, "ValueFrom" may appear either in:
//  - tool.Root.Inputs[i].inputBinding.ValueFrom, OR
//  - tool.OriginalStep.In[i].ValueFrom
// need to handle BOTH cases - first eval at the workflowStepInput level, then eval at the tool input level
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
func (tool *Tool) buildStepInputMap() {
	tool.StepInputMap = make(map[string]*cwl.StepInput)
	for _, in := range tool.OriginalStep.In {
		localID := GetLastInPath(in.ID) // e.g., "file_array" instead of "#subworkflow_test.cwl/test_expr/file_array"
		tool.StepInputMap[localID] = &in
	}
}

// GetLastInPath is a utility function. Example i/o:
// in: "#subworkflow_test.cwl/test_expr/file_array"
// out: "file_array"
func GetLastInPath(s string) (localID string) {
	tmp := strings.Split(s, "/")
	return tmp[len(tmp)-1]
}

/*
explanation for PreProcessContext():

	problem: setting variable name in a js vm to a golang struct doesn't work

	suggested solution by otto examples/docs: use otto.ToValue()

	NOTE: otto library does NOT handle case of converting "complex data types" (e.g., structs) to js objects
	see `func (self *_runtime) toValue(value interface{})` in `runtime.go` in otto library
	comment from otto developer in the typeSwitch in the *_runtime.toValue() method:
	"
	case Object, *Object, _object, *_object:
		// Nothing happens.
		// FIXME We should really figure out what can come here.
		// This catch-all is ugly.
	"

	that means we need to preprocess structs (to a map or array of maps (?)) before loading to vm

	real solution: marshal any struct into json, and then unmarshal that json into a map[string]interface{}
	set the variable in the vm to this map
	this works, and is a simple, elegant solution
	need to update cwl.go function that loads context to InputsVM
	way better to do this json marshal/unmarshal than to handle individual cases
	could suggest this to the otto developer to fix his object handling dilemma
*/

// PreProcessContext is a utility function to preprocess any struct/array before loading into js vm (see above note)
// NOTE: using this json marshalling/unmarshalling strategy implies that the struct field names
// ----- get loaded into the js vm as their json representation.
// ----- this means we can use the cwl fields as json aliases for any struct type's fields
// ----- and then using this function to preprocess the struct/array, all the keys/data will get loaded in properly
// ----- which saves us from having to handle special cases
func PreProcessContext(in interface{}) (out interface{}, err error) {
	j, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(j, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
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
		var ok bool
		if out, ok = tool.Parameters[input.ID]; !ok {
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
	fmt.Println("Here is what usually we work with:")
	PrintJSON(out)

	// if file, need to ensure that all file attributes get populated (e.g., basename)
	if isFile(out) {
		fmt.Println("is a file object")
		path, err := getPath(out)
		if err != nil {
			return nil, err
		}
		out = getFileObj(path.(string))
	} else {
		fmt.Println("is not a file object")
	}

	fmt.Println("after creating file object:")
	PrintJSON(out)

	// at this point, variable `out` is the transformed input thus far (even if no transformation actually occured)
	// so `out` will be what we work with in this next block as an initial value
	// tool inputBinding ValueFrom case
	// Commenting out because the way commands are generated doesn't really handle js expressions
	// See cwl.go/inputs.go/flatten() and Flatten() - this is used to generate commands for CLT's
	// hopefully we can still use this - but maybe need to write our own method to generate commands :/
	if input.Binding != nil && input.Binding.ValueFrom != nil {
		// NOTE: ValueFrom here should be an expression but already has been stripped of $(...) - come on now
		valueFrom := input.Binding.ValueFrom.String
		fmt.Println("here is valueFrom:")
		fmt.Println(valueFrom)
		if strings.HasPrefix(valueFrom, "$") {
			vm := otto.New()
			var context interface{}
			if _, f := out.(*File); f {
				fmt.Println("context is a file")
				context, err = PreProcessContext(out)
				if err != nil {
					return nil, err
				}
			} else {
				fmt.Println("context is not a file")
				context = out
			}
			vm.Set("self", context) // again, will more than likely need additional context here to cover other cases
			if out, err = EvalExpression(valueFrom, vm); err != nil {
				return nil, err
			}
		} else {
			// not an expression, so no eval necessary - take raw value
			out = valueFrom
		}
	}
	fmt.Println("here is after that commented out section")
	PrintJSON(out)

	// note: this initdir_test case sheds light on a problem with how cwl.go's library handles generating commands for CommandLineTools
	// we may very well may need to write our own set of functions for generating commands
	// HERE TODO Thursday: investigate implementation details of how commands are generated
	// --- assess whether we can fix the existing function
	// --- or if we have to write a new function to properly handle generating commands for a commandlinetool
	// also note changes made to cwl.go library - push these changes

	// fmt.Println("Here's tranformed input:")
	// PrintJSON(out)
	return out, nil
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

// LoadVM loads tool.Root.InputsVM with inputs context - using Input.Provided for each input
// to allow js expressions to be evaluated
// TODO: fix loading of inputs to InputsVM
// ----- use PreProcessContext instead of cwl.go's loading method
// ----- because cwl.go's method is super janky and not robust at all
// NOTE: tool.Root.Inputs.ToJavaScriptVM() doesn't work for general usage
// ----- so this function properly loads the inputs context
func (tool *Tool) inputsToVM() (err error) {
	prefix := tool.Root.ID + "/" // need to trim this from all the input.ID's
	fmt.Println("loading inputs to vm..")
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
	fmt.Println("Here's the context")
	PrintJSON(context)
	tool.Root.InputsVM.Set("inputs", context)
	return nil
}

// CollectOutput collects the output for a tool after the tool has run
// output parameter values get set, and the outputs parameter object gets stored in proc.Task.Outputs
// if the outputs of this process are the inputs of another process,
// then the output parameter object of this process (the Task.Outputs field)
// gets assigned as the input parameter object of that other process (the Task.Parameters field)
// ---
// may be a good idea to make different types for CLT and ExpressionTool
// and use Tool as an interface, so we wouldn't have to split cases like this
//  -> could just call one method in one line on a tool interface
// i.e., CollectOutput() should be a method on type CommandLineTool and on type ExpressionTool
// would bypass all this case-handling
// TODO: implement CommandLineTool and ExpressionTool types and their methods, as well as the Tool interface
// ---
// NOTE: the outputBinding for a given output parameter specifies how to assign a value to this parameter
// need to investigate/handle case when there is no outputBinding specified
// - if a CLT, or if multiple output values or multiple output parameters -
// how would output get collected? I feel this must be an error in the given cwl if this happens
func (proc *Process) CollectOutput() (err error) {
	proc.Task.Outputs = make(map[string]cwl.Parameter)
	fmt.Println("collecting output..")
	switch class := proc.Tool.Root.Class; class {
	case "CommandLineTool":
		fmt.Println("Handling CLT output..")
		if err = proc.HandleCLTOutput(); err != nil {
			fmt.Printf("Error handling CLT output: %v\n", err)
			return err
		}
		fmt.Println("CLT outputs:")
	case "ExpressionTool":
		if err = proc.HandleETOutput(); err != nil {
			return err
		}
		fmt.Println("ExpressionTool outputs:")
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	PrintJSON(proc.Task.Outputs)
	return nil
}

// HandleCLTOutput assigns values to output parameters for this CommandLineTool
// stores resulting output parameters object in proc.Task.Outputs
// From my CWL reading.. each output parameter SHOULD have a binding
// if no binding, not sure what the procedure is
// for  now, no binding -> output won't be collected
//
// using dir "/Users/mattgarvin/_fakes3/testWorkflow/initdir_test.cwl" for testing locally
func (proc *Process) HandleCLTOutput() (err error) {
	for _, output := range proc.Task.Root.Outputs {
		if output.Binding == nil {
			return fmt.Errorf("output parameter missing binding: %v", output.ID)
		}

		/*
			Steps for handling CommandLineTool output files (in this order):
			1. Glob everything in the glob list [glob implies File or array of Files output]  (good - see prefix issue)
			2. loadContents (good - see prefix issue)
			3. outputEval (good - need to test)
			4. secondaryFiles (good - need to test expression case)
		*/

		//// Begin 4 step pipeline for collecting/handling CommandLineTool output files ////
		var results []*File

		// 1. Glob - good - need to handle glob pattern prefix issue
		if len(output.Binding.Glob) > 0 {
			results, err = proc.Glob(&output)
			if err != nil {
				return err
			}
		}

		// 2. Load Contents - good - may need to handle same prefix issue

		// uncomment to test LoadContents functionality:
		// output.Binding.LoadContents = true
		if output.Binding.LoadContents {
			for _, fileObj := range results {
				err = fileObj.loadContents()
				if err != nil {
					fmt.Printf("error loading contents: %v\n", err)
					return err
				}
			}
		}

		// 3. OutputEval - good - TODO: test this functionality
		if output.Binding.Eval != nil {
			// eval the expression and store result in task.Outputs
			proc.outputEval(&output, results)
			// if outputEval, then the resulting value from the expression eval is assigned to the output parameter
			// hence the function HandleCLTOutput() returns here
			return nil
		}

		// 4. SecondaryFiles - okay - currently only supporting simplest case when handling expressions here
		if len(output.SecondaryFiles) > 0 {
			for _, entry := range output.SecondaryFiles {
				// see the secondaryFiles field description at:
				// https://www.commonwl.org/v1.0/CommandLineTool.html#CommandOutputParameter
				val := entry.Entry
				if strings.HasPrefix(val, "$") {

					// get inputs context
					vm := proc.Tool.Root.InputsVM.Copy()

					// iterate through output files
					var self interface{}
					for _, fileObj := range results {
						/*
							NOTE: presently only supporting the case of the expression returning a string filepath
							----- NOT supporting the case in which the expression returns either file object or array of file objects
							----- why the flip would the CWL spec allow the option of returning either a string or an object or array of either of these
							----- can extend code here to handle other cases as needed, or when I find an example to work with
						*/

						// preprocess output file object
						self, err = PreProcessContext(fileObj)

						// set `self` variable name
						// assuming it is okay to use one vm for all evaluations and just reset the `self` variable before each eval
						vm.Set("self", self)

						// eval js
						jsResult, err := vm.Run(val)
						if err != nil {
							return err
						}

						// retrieve secondaryFile's path (type interface{} with underlying type string)
						sFilePath, err := jsResult.Export()
						if err != nil {
							return err
						}

						// TODO: check if resulting secondaryFile actually exists (should encapsulate this to a function)

						// get file object for secondaryFile and append it to the output file's SecondaryFiles field
						sFileObj := getFileObj(sFilePath.(string))
						fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFileObj)
					}

				} else {
					// follow those two steps indicated at the bottom of the secondaryFiles field description
					suffix, carats := trimLeading(val, "^")
					if err != nil {
						return err
					}
					for _, fileObj := range results {
						fileObj.loadSFilesFromPattern(suffix, carats)
					}
				}
			}
		}
		//// end of 4 step processing pipeline for collecting/handling output files ////

		// at this point we have file results captured in `results`
		// output should be a "File" or "array of Files"
		if output.Types[0].Type == "File" {
			// TODO: add error handling for cases len(results) != 1
			proc.Task.Outputs[output.ID] = results[0]
		} else {
			// output should be an array of File objects
			// also need to add error handling here
			proc.Task.Outputs[output.ID] = results
		}
	}
	return nil
}

// creates File object for secondaryFile and loads into fileObj.SecondaryFiles field
// unsure of where/what to check here to potentially return an error
func (fileObj *File) loadSFilesFromPattern(suffix string, carats int) (err error) {
	path := fileObj.Location
	// however many chars there are
	// trim that number of file extentions from the end of the path
	for i := 0; i < carats; i++ {
		tmp := strings.Split(path, ".") // split path at periods
		tmp = tmp[:len(tmp)-1]          // exclude last file extension
		path = strings.Join(tmp, ".")   // reconstruct path without last file extension
	}
	path = path + suffix // append suffix (which is the original pattern with leading carats trimmed)

	// check whether file exists
	fileExists, err := exists(path)
	// HERE - TODO - decide how to handle case of secondaryFiles that don't exist - warning or error? still append file obj to list or not?
	// see: https://www.commonwl.org/v1.0/Workflow.html#WorkflowOutputParameter
	switch {
	case fileExists:
		// the secondaryFile exists
		fmt.Printf("\tfound secondary file %v\n", path)

		// construct file object for this secondary file
		sFile := getFileObj(path)

		// append this secondary file
		fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFile)

	case !fileExists:
		// the secondaryFile does not exist
		// if anything, this should be a warning - not an error
		// presently in this case, the secondaryFile object does NOT get appended to fileObj.SecondaryFiles
		fmt.Printf("\tWARNING: secondary file %v not found\n", path)
	}
	return nil
}

// exists returns whether the given file or directory exists
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

// given a string s and a character char
// count number of leading char's
// return s trimmed of leading char and count number of char's trimmed
func trimLeading(s string, char string) (suffix string, count int) {
	count = 0
	prevChar := char
	for i := 0; i < len(s) && prevChar == char; i++ {
		prevChar = string(s[i])
		if prevChar == char {
			count++
		}
	}
	suffix = strings.TrimLeft(s, char)
	return suffix, count
}

// see: https://www.commonwl.org/v1.0/Workflow.html#CommandOutputBinding
func (proc *Process) outputEval(output *cwl.Output, fileArray []*File) (err error) {
	// copy InputsVM to get inputs context
	vm := proc.Tool.Root.InputsVM.Copy()

	// here `self` is the file or array of files returned by glob (with contents loaded if so specified)
	var self interface{}
	if output.Types[0].Type == "File" {
		// indicates `self` should be a file object with keys exposed
		// should check length fileArray - room for error here
		self, err = PreProcessContext(fileArray[0])
		if err != nil {
			return err
		}
	} else {
		// Not "File" means "array of Files"
		self, err = PreProcessContext(fileArray)
		if err != nil {
			return err
		}
	}

	// set `self` var in the vm
	vm.Set("self", self)

	// get outputEval expression
	expression := output.Binding.Eval.Raw

	// eval that thing
	evalResult, err := EvalExpression(expression, vm)
	if err != nil {
		return err
	}

	// assign expression eval result to output parameter
	proc.Task.Outputs[output.ID] = evalResult
	return nil
}

// loads contents of file into the File.Contents field
// NOTE: need handle prefix issue
func (fileObj *File) loadContents() (err error) {
	// HERE TODO same path prefix issue as in Glob() needs to be handled
	// prefix depends bucket mount location in workflow engine container and folder structure of bucket
	prefix := ""
	r, err := os.Open(prefix + fileObj.Location)
	if err != nil {
		return err
	}
	// read up to 64 KiB from file, as specified in CWL docs
	// 1 KiB is 1024 bytes -> 64 KiB is 65536 bytes
	contents := make([]byte, 65536, 65536)
	_, err = r.Read(contents)
	if err != nil {
		fmt.Printf("error reading file contents: %v", err)
		return err
	}
	// trim trailing null bytes if less than 65536 bytes were read
	contents = bytes.TrimRight(contents, "\u0000")

	// populate File.Contents field with contents
	fileObj.Contents = string(contents)
	return nil
}

// Glob collects output file(s) for a CLT output parameter after that CLT has run
// returns an array of files
func (proc *Process) Glob(output *cwl.Output) (results []*File, err error) {
	var pattern string
	for _, glob := range output.Binding.Glob {
		pattern, err = proc.getPattern(glob)
		if err != nil {
			return results, err
		}
		/*
			where exactly should we be globbing?
			there should be some kind of prefix
			like "{mount_point}/workflows/{jobID}/{stepID}/" or something
			prefix depends on:
			1. fuse mount location
			2. any intermediate path-walking to get to the `workflows` dir
			3. the top-level workflow job get its own dir
			4. each step gets its own dir in the top-level workflow dir
			5. if there is an InitialWorkDirRequirement (https://www.commonwl.org/v1.0/Workflow.html#InitialWorkDirRequirement)
		*/

		// currently using this directory to test the workflow output collection/globbing
		var prefix string
		if proc.Task.ScatterIndex != 0 {
			// NOTE: each scattered subtask of a scattered task will have its own working dir
			prefix = fmt.Sprintf("/Users/mattgarvin/_fakes3/testWorkflow/%v-scatter-%v/", proc.Task.Root.ID, proc.Task.ScatterIndex)
		} else {
			prefix = fmt.Sprintf("/Users/mattgarvin/_fakes3/testWorkflow/%v/", proc.Task.Root.ID)
		}
		paths, err := filepath.Glob(prefix + pattern)
		if err != nil {
			return results, err
		}
		for _, path := range paths {
			fileObj := getFileObj(path)
			results = append(results, fileObj)
		}
	}
	return results, nil
}

func (proc *Process) getPattern(glob string) (pattern string, err error) {
	if strings.HasPrefix(glob, "$") {
		// expression needs to get eval'd
		// glob pattern is the resulting string
		// eval'ing in the InputsVM with no additional context
		// not sure if additional context will need to be added in other cases

		// HERE - $(inputs.input_bam.basename) fails to eval in VM
		// what's the deal
		inputs, _ := proc.Tool.Root.InputsVM.Run("inputs")
		seeInputs, _ := inputs.Export()
		fmt.Println("here is inputs in the vm:")
		PrintJSON(seeInputs)

		expResult, err := EvalExpression(glob, proc.Tool.Root.InputsVM)
		if err != nil {
			return "", fmt.Errorf("failed to eval glob expression: %v", glob)
		}
		pattern, ok := expResult.(string)
		if !ok {
			return "", fmt.Errorf("glob expression doesn't return a string pattern: %v", glob)
		}
		return pattern, nil
	}
	// not an expression, so no eval necessary
	// glob pattern is the glob string initially provided
	return glob, nil
}

// HandleETOutput ..
// ExpressionTool expression returns a JSON object
// where the keys are the IDs of the expressionTool output params
// see `expression` field description here:
// https://www.commonwl.org/v1.0/Workflow.html#ExpressionTool
func (proc *Process) HandleETOutput() (err error) {
	for _, output := range proc.Task.Root.Outputs {
		// get "output" from "#expressiontool_test.cwl/output"
		localOutputID := GetLastInPath(output.ID)

		// access output param value from expression result
		val, ok := proc.Tool.ExpressionResult[localOutputID]
		if !ok {
			return fmt.Errorf("output parameter %v missing from ExpressionTool %v result", output.ID, proc.Task.Root.ID)
		}

		// assign retrieved value to output param in Task object
		proc.Task.Outputs[output.ID] = val
	}
	return nil
}

// RunTool runs the tool
// If ExpressionTool, passes to appropriate handler to eval the expression
// If CommandLineTool, passes to appropriate handler to create k8s job
func (engine *K8sEngine) runTool(proc *Process) (err error) {
	switch class := proc.Tool.Root.Class; class {
	case "ExpressionTool":
		err = engine.RunExpressionTool(proc)
		if err != nil {
			return err
		}

		proc.CollectOutput()

		// JS gets evaluated in-line, so the process is complete when the engine method RunExpressionTool() returns
		delete(engine.UnfinishedProcs, proc.Tool.Root.ID)
		engine.FinishedProcs[proc.Tool.Root.ID] = proc

	case "CommandLineTool":
		err = engine.RunCommandLineTool(proc)
		if err != nil {
			return err
		}
		err = engine.ListenForDone(proc) // tells engine to listen to k8s to check for this process to finish running
		if err != nil {
			return fmt.Errorf("error listening for done: %v", err)
		}
	default:
		return fmt.Errorf("unexpected class: %v", class)
	}
	return nil
}

// RunCommandLineTool runs a CommandLineTool
func (engine K8sEngine) RunCommandLineTool(proc *Process) (err error) {
	fmt.Println("\tRunning CommandLineTool")
	err = proc.Tool.GenerateCommand() // need to test different cases of generating commands
	if err != nil {
		return err
	}
	err = engine.RunK8sJob(proc) // push Process struct onto engine.UnfinishedProcs
	if err != nil {
		return err
	}
	return nil
}

// RunExpressionTool runs an ExpressionTool
func (engine *K8sEngine) RunExpressionTool(proc *Process) (err error) {
	// note: context has already been loaded
	result, err := EvalExpression(proc.Tool.Root.Expression, proc.Tool.Root.InputsVM)
	if err != nil {
		return err
	}

	// expression must return a JSON object where the keys are the IDs of the ExpressionTool outputs
	// see description of `expression` field here:
	// https://www.commonwl.org/v1.0/Workflow.html#ExpressionTool
	var ok bool
	proc.Tool.ExpressionResult, ok = result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expressionTool expression did not return a JSON object")
	}
	return nil
}

// GetJS strips the cwl prefix for an expression
// and tells whether to just eval the expression, or eval the exp as a js function
// this is modified from the cwl.Eval.ToJavaScriptString() method
func GetJS(s string) (js string, fn bool, err error) {
	// if curly braces, then need to eval as a js function
	// see https://www.commonwl.org/v1.0/Workflow.html#Expressions
	fn = strings.HasPrefix(s, "${")
	s = strings.TrimLeft(s, "$(\n")
	s = strings.TrimRight(s, ")\n")
	return s, fn, nil
}

// EvalExpression is an engine for handling in-line js in cwl
// the exp is passed before being stripped of any $(...) or ${...} wrapper
// the vm must be loaded with all necessary context for eval
// EvalExpression handles parameter references and expressions $(...), as well as functions ${...}
func EvalExpression(exp string, vm *otto.Otto) (result interface{}, err error) {
	// strip the $() (or if ${} just trim leading $), which appears in the cwl as a wrapper for js expressions
	var output otto.Value
	js, fn, _ := GetJS(exp)
	if js == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if fn {
		// if expression wrapped like ${...}, need to run as a zero arg js function

		// construct js function definition
		fnDef := fmt.Sprintf("function f() %s", js)
		// fmt.Printf("Here's the fnDef:\n%v\n", fnDef)

		// run this function definition so the function exists in the vm
		vm.Run(fnDef)

		// call this function in the vm
		output, err = vm.Run("f()")
		if err != nil {
			fmt.Printf("\terror running js function: %v\n", err)
			return nil, err
		}
	} else {
		output, err = vm.Run(js)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate js expression: %v", err)
		}
	}
	result, _ = output.Export()
	return result, nil
}

func (tool *Tool) setupTool() (err error) {
	err = tool.loadInputs() // pass parameter values to input.Provided for each input
	if err != nil {
		fmt.Printf("\tError loading inputs: %v\n", err)
		return err
	}
	err = tool.inputsToVM() // loads inputs context to js vm tool.Root.InputsVM (Ready to test, but needs to be extended)
	if err != nil {
		fmt.Printf("\tError loading inputs to js VM: %v\n", err)
		return err
	}
	err = tool.initWorkDir()
	if err != nil {
		return err
	}
	return nil
}

// determines whether i represents a CWL file object
// if file, then returns path, true
// ow returns "", false
// see initWorkDir()
// TODO - have this function return path - see getPath()
func isFile(i interface{}) (f bool) {
	fmt.Println("checking if file..")
	iType := reflect.TypeOf(i)
	fmt.Println("type:")
	PrintJSON(iType)
	iKind := iType.Kind()
	fmt.Println("kind:")
	PrintJSON(iKind)
	if iKind == reflect.Map {
		fmt.Println("this is a map")
		iMap := reflect.ValueOf(i)
		for _, key := range iMap.MapKeys() {
			if key.Type() == reflect.TypeOf("") {
				fmt.Println("found a string")
				if key.String() == "class" {
					fmt.Println("found class")
					PrintJSON(iMap.MapIndex(key).Interface())
					if iMap.MapIndex(key).Interface() == "File" {
						f = true
					}
				}
			}
		}
	} else {
		fmt.Println("this is not a map")
	}
	return f
}

// get path from a file object which is not of type File
// NOTE: maybe shouldn't be an error, if the contents field is populated
func getPath(i interface{}) (path interface{}, err error) {
	iter := reflect.ValueOf(i).MapRange()
	for iter.Next() {
		key, val := iter.Key().String(), iter.Value()
		if key == "location" || key == "path" {
			return val.Interface(), nil
		}
	}
	return "", fmt.Errorf("no location or path specified")
}

// initDirReq handles the InitialWorkDirRequirement if specified for this tool
// TODO: handle prefix issue; support cases where File or dirent is returned from `entry`
// NOTE: this function really needs to be cleaned up/revised
func (tool *Tool) initWorkDir() (err error) {
	var result, resFile interface{}

	for _, requirement := range tool.Root.Requirements {
		if requirement.Class == "InitialWorkDirRequirement" {
			fmt.Println("found InitialWorkDirRequirement:")
			PrintJSON(requirement)
			for _, listing := range requirement.Listing {
				// handling the case where `entry` is content (expression or string) to be written to a file
				// and `entryname` is the name of the file to be created
				var contents interface{}
				if strings.HasPrefix(listing.Entry, "$") {
					// `entry` is an expression which may return a string, File or `dirent`
					// NOTE: presently NOT supporting the File or dirent case
					// what's a dirent? good question: https://www.commonwl.org/v1.0/CommandLineTool.html#Dirent
					result, err = EvalExpression(listing.Entry, tool.Root.InputsVM)
					if err != nil {
						return err
					}
					fmt.Printf("entry expression: %v\n", listing.Entry)
					fmt.Println("result of entry expression:")
					PrintJSON(result)
					/*
						// to handle case where result is a file object
						// presently writing whatever the expression returns to the newly created file
						if isFile(result) {
							resFile = result
						} else {
							contents = result
						}
					*/
					contents = result
				} else {
					contents = listing.Entry
				}
				PrintJSON(contents)

				// `entryName` for sure is a string literal or an expression which evaluates to a string
				// `entryName` is the name of the file to be created
				var entryName string
				if strings.HasPrefix(listing.EntryName, "$") {
					result, err = EvalExpression(listing.EntryName, tool.Root.InputsVM)
					if err != nil {
						return err
					}
					var ok bool
					entryName, ok = result.(string)
					if !ok {
						return fmt.Errorf("entryname expression did not return a string")
					}
				} else {
					entryName = listing.EntryName
				}

				/*
					Cases:
					1. `entry` returned a file object - file object stored as an interface{} in `resFile` (NOT SUPPORTED)
					2. `entry` did not return a file object - then returned value is in `contents` and must be written to a new file with filename stored in `entryName` (supported)
				*/

				// HERE - TODO: handle same prefix issue as for collecting CLT output - see HandleCLTOutput()
				prefix := "/Users/mattgarvin/_fakes3/testWorkflow/#initdir_test.cwl" // for testing
				if resFile != nil {
					// "If the value is an expression that evaluates to a File object,
					// this indicates the referenced file should be added to the designated output directory prior to executing the tool."
					// NOTE: the "designated output directory" is just the directory corresponding to the *Tool
					// not sure what the purpose/meaning/use of this feature is - pretty sure all i/o for *Tools gets handled already
					// presently not supporting this case - will implement this feature once I find an example to work with
					panic("feature not supported: entry expression returned a file object")
				} else {
					jContents, err := json.Marshal(contents)
					if err != nil {
						return err
					}
					f, err := os.Create(filepath.Join(prefix, entryName))
					if err != nil {
						return err
					}
					f.Write(jContents)
					f.Close()
				}
			}
		}
	}
	return nil
}

// DispatchTask does some setup for and dispatches workflow *Tools - i.e., CommandLineTools and ExpressionTools
func (engine K8sEngine) DispatchTask(jobID string, task *Task) (err error) {
	tool := task.getTool()
	err = tool.setupTool()
	// fmt.Printf("here are the requirements for tool %v\n", task.Root.ID)
	// PrintJSON(tool.Root.Requirements)

	// NOTE: there's a lot of duplicated information here, because Tool is almost a subset of Task
	// this will be handled when code is refactored/polished/cleaned up
	proc := &Process{
		Tool: tool,
		Task: task,
	}

	// (when should the process get pushed onto the stack?)
	// push newly started process onto the engine's stack of running processes
	engine.UnfinishedProcs[tool.Root.ID] = proc

	// engine runs the tool either as a CommandLineTool or ExpressionTool
	err = engine.runTool(proc)
	if err != nil {
		fmt.Printf("\tError running tool: %v\n", err)
		return err
	}
	return nil
}
