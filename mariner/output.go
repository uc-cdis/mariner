package mariner

import (
	"os"
	"path/filepath"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for collecting/processing output from Tools

// CollectOutput collects the output for a tool after the tool has run
// output parameter values get set, and the outputs parameter object gets stored in tool.Task.Outputs
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
// ----- no binding provided -> output won't be collected
func (tool *Tool) collectOutput() (err error) {
	tool.Task.infof("begin collect output")
	switch class := tool.Task.Root.Class; class {
	case CWLCommandLineTool:
		if err = tool.handleCLTOutput(); err != nil {
			return tool.Task.errorf("%v", err)
		}
	case CWLExpressionTool:
		if err = tool.handleETOutput(); err != nil {
			return tool.Task.errorf("%v", err)
		}
	default:
		return tool.Task.errorf("unexpected class: %v", class)
	}
	tool.Task.infof("end collect outputt")
	return nil
}

// HandleCLTOutput assigns values to output parameters for this CommandLineTool
// stores resulting output parameters object in tool.Task.Outputs
// From my CWL reading.. each output parameter SHOULD have a binding
// if no binding, not sure what the procedure is
// for now, no binding -> output won't be collected
//
// fixme: refactor, break into smaller pieces/functions
func (tool *Tool) handleCLTOutput() (err error) {
	tool.Task.infof("begin handle CommandLineTool output")
	for _, output := range tool.Task.Root.Outputs {
		tool.Task.infof("begin handle output param: %v", output.ID)
		if output.Binding == nil {
			return tool.Task.errorf("binding not found")
		}

		/*
			Steps for handling CommandLineTool output files (in this order):
			1. Glob everything in the glob list [glob implies File or array of Files output]
			2. loadContents
			3. outputEval
			4. secondaryFiles
		*/

		//// Begin 4 step pipeline for collecting/handling CommandLineTool output files ////
		var results []*File

		// 1. Glob - prefixissue
		if len(output.Binding.Glob) > 0 {
			results, err = tool.glob(&output)
			if err != nil {
				return tool.Task.errorf("%v", err)
			}
		}

		// 2. Load Contents
		// no need to handle prefixes here, since the full paths
		// are already in the File objects stored in `results`
		if output.Binding.LoadContents {
			tool.Task.infof("begin load file contents")
			for _, fileObj := range results {
				tool.Task.infof("begin load contents for file :%v", fileObj.Path)
				err = fileObj.loadContents()
				if err != nil {
					return tool.Task.errorf("%v", err)
				}
				tool.Task.infof("end load contents for file :%v", fileObj.Path)
			}
			tool.Task.infof("end load file contents")
		}

		// 3. OutputEval - TODO: test this functionality
		if output.Binding.Eval != nil {
			// eval the expression and store result in task.Outputs
			if err = tool.outputEval(&output, results); err != nil {
				return tool.Task.errorf("%v", err)
			}
			// if outputEval, then the resulting value from the expression eval is assigned to the output parameter
			// hence the function HandleCLTOutput() returns here
			return nil
		}

		// 4. SecondaryFiles - currently only supporting simplest case when handling expressions here
		if len(output.SecondaryFiles) > 0 {
			tool.Task.infof("begin handle secondaryFiles")
			for _, entry := range output.SecondaryFiles {
				// see the secondaryFiles field description at:
				// https://www.commonwl.org/v1.0/CommandLineTool.html#CommandOutputParameter
				val := entry.Entry
				if strings.HasPrefix(val, "$") {

					// get inputs context
					vm := tool.Task.Root.InputsVM.Copy()

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
						self, err = preProcessContext(fileObj)
						if err != nil {
							return tool.Task.errorf("%v", err)
						}

						// set `self` variable name
						// assuming it is okay to use one vm for all evaluations and just reset the `self` variable before each eval
						vm.Set("self", self)

						// eval js
						jsResult, err := evalExpression(val, vm)
						if err != nil {
							return tool.Task.errorf("%v", err)
						}

						// retrieve secondaryFile's path (type interface{} with underlying type string)
						sFilePath, ok := jsResult.(string)
						if !ok {
							return tool.Task.errorf("secondaryFile expression did not return string")
						}

						// TODO: check if resulting secondaryFile actually exists (should encapsulate this to a function)

						// get file object for secondaryFile and append it to the output file's SecondaryFiles field
						sFileObj := fileObject(sFilePath)
						fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFileObj)
					}

				} else {
					// follow those two steps indicated at the bottom of the secondaryFiles field description
					suffix, carats := trimLeading(val, "^")
					for _, fileObj := range results {
						tool.loadSFilesFromPattern(fileObj, suffix, carats)
					}
				}
			}
			tool.Task.infof("end handle secondaryFiles")
		}
		//// end of 4 step processing pipeline for collecting/handling output files ////

		// at this point we have file results captured in `results`
		// output should be a CWLFileType or "array of Files"
		// fixme - make this case handling more specific in the else condition - don't just catch anything
		if output.Types[0].Type == CWLFileType {
			// fmt.Println("output type is file")

			// fixme - add error handling for cases len(results) != 1
			tool.Task.Outputs.update(output.ID, results[0])
		} else {
			// output should be an array of File objects
			// note: also need to add error handling here
			tool.Task.Outputs.update(output.ID, results)
		}
		tool.Task.infof("end handle output param: %v", output.ID)
	}
	tool.Task.infof("end handle CommandLineTool output")
	return nil
}

// Glob collects output file(s) for a CLT output parameter after that CLT has run
// returns an array of files
func (tool *Tool) glob(output *cwl.Output) (results []*File, err error) {
	tool.Task.infof("begin glob")

	os.Chdir("/") // always glob from root (?)

	var pattern string
	for _, glob := range output.Binding.Glob {
		pattern, err = tool.pattern(glob)
		if err != nil {
			return results, tool.Task.errorf("%v", err)
		}
		paths, err := filepath.Glob(tool.WorkingDir + pattern)
		if err != nil {
			return results, tool.Task.errorf("%v", err)
		}
		for _, path := range paths {
			fileObj := fileObject(path) // these are full paths, so no need to add working dir to path
			results = append(results, fileObj)
		}
	}
	os.Chdir(tool.WorkingDir)
	tool.Task.infof("end glob")
	return results, nil
}

func (tool *Tool) pattern(glob string) (pattern string, err error) {
	tool.Task.infof("begin resolve glob pattern: %v", glob)
	if strings.HasPrefix(glob, "$") {
		// expression needs to get eval'd
		// glob pattern is the resulting string
		// eval'ing in the InputsVM with no additional context
		// not sure if additional context will be needed in other cases

		expResult, err := evalExpression(glob, tool.Task.Root.InputsVM)
		if err != nil {
			return "", tool.Task.errorf("failed to eval glob expression")
		}
		pattern, ok := expResult.(string)
		if !ok {
			return "", tool.Task.errorf("glob expression doesn't return a string pattern")
		}
		return pattern, nil
	}
	// not an expression, so no eval necessary
	// glob pattern is the glob string initially provided
	tool.Task.infof("end resolve glob pattern. resolved to: %v", glob)
	return glob, nil
}

// HandleETOutput ..
// ExpressionTool expression returns a JSON object
// where the keys are the IDs of the expressionTool output params
// see `expression` field description here:
// https://www.commonwl.org/v1.0/Workflow.html#ExpressionTool
func (tool *Tool) handleETOutput() error {
	tool.Task.infof("begin handle ExpressionTool output")
	for _, output := range tool.Task.Root.Outputs {
		// get "output" from "#expressiontool_test.cwl/output"
		localOutputID := lastInPath(output.ID)

		// access output param value from expression result
		val := tool.ExpressionResult.read(localOutputID)
		if val == nil {
			return tool.Task.errorf("no value found for output parameter %v", output.ID)
		}

		// assign retrieved value to output param in Task object
		tool.Task.Outputs.update(output.ID, val)
	}
	tool.Task.infof("end handle ExpressionTool output")
	return nil
}

// see: https://www.commonwl.org/v1.0/Workflow.html#CommandOutputBinding
func (tool *Tool) outputEval(output *cwl.Output, fileArray []*File) (err error) {
	tool.Task.infof("begin output eval for output param %v", output.ID)
	// copy InputsVM to get inputs context
	vm := tool.Task.Root.InputsVM.Copy()

	// here `self` is the file or array of files returned by glob (with contents loaded if so specified)
	var self interface{}
	if output.Types[0].Type == CWLFileType {
		// indicates `self` should be a file object with keys exposed
		// should check length fileArray - room for error here
		self, err = preProcessContext(fileArray[0])
		if err != nil {
			return tool.Task.errorf("%v", err)
		}
	} else {
		// Not CWLFileType means "array of Files"
		self, err = preProcessContext(fileArray)
		if err != nil {
			return tool.Task.errorf("%v", err)
		}
	}

	// set `self` var in the vm
	vm.Set("self", self)

	// get outputEval expression
	expression := output.Binding.Eval.Raw

	// eval that thing
	evalResult, err := evalExpression(expression, vm)
	if err != nil {
		return tool.Task.errorf("%v", err)
	}

	// assign expression eval result to output parameter
	tool.Task.Outputs.update(output.ID, evalResult)

	tool.Task.infof("end output eval for output param %v", output.ID)
	return nil
}
