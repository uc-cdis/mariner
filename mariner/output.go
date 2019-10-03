package mariner

import (
	"fmt"
	"path/filepath"
	"strings"

	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for collecting/processing output from *Tools

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
// ----- no binding provided -> output won't be collected
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
	// PrintJSON(proc.Task.Outputs)
	return nil
}

// HandleCLTOutput assigns values to output parameters for this CommandLineTool
// stores resulting output parameters object in proc.Task.Outputs
// From my CWL reading.. each output parameter SHOULD have a binding
// if no binding, not sure what the procedure is
// for now, no binding -> output won't be collected
func (proc *Process) HandleCLTOutput() (err error) {
	for _, output := range proc.Task.Root.Outputs {
		fmt.Println("handling CLT output for output")
		PrintJSON(output)
		if output.Binding == nil {
			return fmt.Errorf("output parameter missing binding: %v", output.ID)
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

		fmt.Println("step 1 - glob")

		// 1. Glob - prefixissue
		if len(output.Binding.Glob) > 0 {
			fmt.Println("globbing with this pattern:")
			PrintJSON(output.Binding.Glob)
			results, err = proc.Glob(&output)
			if err != nil {
				fmt.Printf("error globbing: %v", err)
				return err
			}
		}

		// 2. Load Contents
		// no need to handle prefixes here, since the full paths
		// are already in the File objects stored in `results`

		fmt.Println("step 2 - load contents")

		// uncomment to test LoadContents functionality:
		// output.Binding.LoadContents = true
		if output.Binding.LoadContents {
			fmt.Println("load contents is true")
			for _, fileObj := range results {
				fmt.Println("loading contents for this file")
				PrintJSON(fileObj)
				err = fileObj.loadContents()
				if err != nil {
					fmt.Printf("error loading contents: %v\n", err)
					return err
				}
			}
		}

		// 3. OutputEval - TODO: test this functionality
		if output.Binding.Eval != nil {
			// eval the expression and store result in task.Outputs
			proc.outputEval(&output, results)
			// if outputEval, then the resulting value from the expression eval is assigned to the output parameter
			// hence the function HandleCLTOutput() returns here
			return nil
		}

		// 4. SecondaryFiles - currently only supporting simplest case when handling expressions here
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
						jsResult, err := EvalExpression(val, vm)
						if err != nil {
							return err
						}

						// retrieve secondaryFile's path (type interface{} with underlying type string)
						sFilePath, ok := jsResult.(string)
						if !ok {
							return fmt.Errorf("secondaryFile expression did not return string")
						}

						// TODO: check if resulting secondaryFile actually exists (should encapsulate this to a function)

						// get file object for secondaryFile and append it to the output file's SecondaryFiles field
						sFileObj := getFileObj(sFilePath)
						fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFileObj)
					}

				} else {
					// follow those two steps indicated at the bottom of the secondaryFiles field description
					suffix, carats := trimLeading(val, "^")
					if err != nil {
						return err
					}
					for _, fileObj := range results {
						proc.Tool.loadSFilesFromPattern(fileObj, suffix, carats)
					}
				}
			}
		}
		//// end of 4 step processing pipeline for collecting/handling output files ////

		fmt.Println("done with glob and load contents")
		fmt.Println("at end of function here")

		fmt.Println("here are results:")
		PrintJSON(results)

		// at this point we have file results captured in `results`
		// output should be a "File" or "array of Files"
		if output.Types[0].Type == "File" {
			fmt.Println("output type is file")

			// TODO: add error handling for cases len(results) != 1
			proc.Task.Outputs[output.ID] = results[0]
		} else {
			// output should be an array of File objects
			// NOTE: also need to add error handling here
			proc.Task.Outputs[output.ID] = results
		}
	}
	return nil
}

// Glob collects output file(s) for a CLT output parameter after that CLT has run
// returns an array of files
func (proc *Process) Glob(output *cwl.Output) (results []*File, err error) {
	var pattern string
	fmt.Println("in proc glob method")
	for _, glob := range output.Binding.Glob {
		pattern, err = proc.getPattern(glob)
		if err != nil {
			return results, err
		}

		fmt.Println("this pattern")
		fmt.Println(pattern)
		fmt.Println("here is working dir")
		fmt.Println(proc.Tool.WorkingDir)

		fmt.Println("globbing here")
		fmt.Println(proc.Tool.WorkingDir + pattern)

		paths, err := filepath.Glob(proc.Tool.WorkingDir + pattern)
		fmt.Println("here are the resulting paths")
		fmt.Println(paths)
		if err != nil {
			fmt.Printf("error globbing: %v", err)
			return results, err
		}
		for _, path := range paths {
			fmt.Println("creating file object for path ", path)
			fileObj := getFileObj(path) // these are full paths, so no need to add working dir to path
			fmt.Println("here is the resulting file object")
			PrintJSON(fileObj)
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
		// not sure if additional context will be needed in other cases

		/*
			inputs, _ := proc.Tool.Root.InputsVM.Run("inputs")
			seeInputs, _ := inputs.Export()
			fmt.Println("here is inputs in the vm:")
			PrintJSON(seeInputs)
		*/

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
