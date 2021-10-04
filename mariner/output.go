package mariner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	log "github.com/sirupsen/logrus"
	cwl "github.com/uc-cdis/cwl.go"
)

// this file contains code for collecting/processing output from Tools

// HandleCLTOutput assigns values to output parameters for this CommandLineTool
// stores resulting output parameters object in tool.Task.Outputs
// From my CWL reading.. each output parameter SHOULD have a binding
// if no binding, not sure what the procedure is
// for now, no binding -> output won't be collected
//
// fixme: refactor, break into smaller pieces/functions
func (engine *K8sEngine) handleCLTOutput(tool *Tool) (err error) {
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
			results, err = engine.glob(tool, &output)
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
				err = engine.loadContents(fileObj)
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
			//return nil
			log.Infof("here is the eval %s", output.Binding.Eval.Raw)
			continue
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
					vm := tool.InputsVM.Copy()

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
						// UPDATE: you can do that with this: engine.fileExists(sFilePath)

						// get file object for secondaryFile and append it to the output file's SecondaryFiles field
						sFileObj := fileObject(sFilePath)
						fileObj.SecondaryFiles = append(fileObj.SecondaryFiles, sFileObj)
					}

				} else {
					// follow those two steps indicated at the bottom of the secondaryFiles field description
					suffix, carats := trimLeading(val, "^")
					for _, fileObj := range results {
						engine.loadSFilesFromPattern(tool, fileObj, suffix, carats)
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

			// fixme - add error handling for cases len(results) != 1
			if len(results) > 0 {
				tool.Task.Outputs[output.ID] = results[0]
			}
		} else {
			tool.Task.Lock()
			// output should be an array of File objects
			// note: also need to add error handling here
			tool.Task.Outputs[output.ID] = results // #race (?)
			tool.Task.Unlock()
		}
		tool.Task.infof("end handle output param: %v", output.ID)
	}
	tool.Task.infof("end handle CommandLineTool output")
	return nil
}

// Glob collects output file(s) for a CLT output parameter after that CLT has run
// returns an array of files
//
// #no-fuse - must glob s3, not locally
func (engine *K8sEngine) glob(tool *Tool, output *cwl.Output) (results []*File, err error) {
	tool.Task.infof("begin glob")
	var pattern string
	var patterns []string
	for _, glob := range output.Binding.Glob {
		pattern, err = tool.pattern(glob)
		if err != nil {
			return results, tool.Task.errorf("%v", err)
		}
		patterns = append(patterns, pattern)
	}
	paths, err := engine.globS3(tool, patterns)
	if err != nil {
		return results, tool.Task.errorf("%v", err)
	}
	for _, path := range paths {
		fileObj := fileObject(path) // these are full paths, so no need to add working dir to path
		results = append(results, fileObj)
	}
	tool.Task.infof("end glob")
	return results, nil
}

/*
	(get list of all files in the tool's working dir)
	s3 ls --recursive <tool_working_dir>

	then filter that list by the glob pattern
	your resulting path list
	consists of all the paths in the working dir which match the pattern

	use this:
	https://golang.org/pkg/path/filepath/#Match
*/
func (engine *K8sEngine) globS3(tool *Tool, patterns []string) ([]string, error) {
	svc := s3.New(engine.S3FileManager.newS3Session())
	objectList, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(engine.S3FileManager.S3BucketName),
		Prefix: aws.String(strings.TrimPrefix(engine.localPathToS3Key(tool.WorkingDir), "/")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list keys from tool working dir in s3: %v", err)
	}

	/*
		note:
		glob patterns in the CWL should (?) be specified relative
		to that task's runtime environment
		that is, all glob patterns should (?) resolve to absolute paths
		the way to do this in the CWL is, for example:
		glob: $(runtime.outdir + 'my_glob_pattern*')

		see also: https://www.commonwl.org/v1.0/CommandLineTool.html#Runtime_environment
	*/

	var key string
	var match bool
	var collectFile bool
	var path string
	globResults := []string{}
	for _, obj := range objectList.Contents {
		// match key against pattern
		key = *obj.Key

		collectFile = false
		for _, pattern := range patterns {
			s3Pattern := strings.TrimPrefix(engine.localPathToS3Key(pattern), "/")

			// handle case of glob pattern not resolving to absolute path
			// fixme: this is not pretty
			if !strings.HasPrefix(s3Pattern, engine.UserID) {
				s3wkdir := strings.TrimPrefix(engine.localPathToS3Key(tool.WorkingDir), "/")
				s3Pattern = fmt.Sprintf("%s/%s", strings.TrimSuffix(s3wkdir, "/"), strings.TrimPrefix(s3Pattern, "/"))
			}

			match, err = filepath.Match(s3Pattern, key)
			if err != nil {
				return nil, fmt.Errorf("glob pattern matching failed: %v", err)
			} else if match {
				collectFile = true
			}
		}
		if collectFile {
			// this needs to be represented as a filepath, not a "key"
			// i.e., it needs a slash at the beginning
			path = engine.s3KeyToLocalPath(fmt.Sprintf("/%s", key))
			globResults = append(globResults, path)
		}
	}
	return globResults, nil
}

func (tool *Tool) pattern(glob string) (pattern string, err error) {
	tool.Task.infof("begin resolve glob pattern: %v", glob)
	if strings.HasPrefix(glob, "$") {
		// expression needs to get eval'd
		// glob pattern is the resulting string
		// eval'ing in the InputsVM with no additional context
		// not sure if additional context will be needed in other cases

		expResult, err := evalExpression(glob, tool.InputsVM)
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
func (engine *K8sEngine) handleETOutput(tool *Tool) error {
	tool.Task.infof("begin handle ExpressionTool output")
	for _, output := range tool.Task.Root.Outputs {
		// get "output" from "#expressiontool_test.cwl/output"
		localOutputID := lastInPath(output.ID)

		// access output param value from expression result
		val, ok := tool.ExpressionResult[localOutputID]
		if !ok {
			return tool.Task.errorf("no value found for output parameter %v", output.ID)
		}

		// assign retrieved value to output param in Task object
		tool.Task.Outputs[output.ID] = val
	}
	tool.Task.infof("end handle ExpressionTool output")
	return nil
}

// see: https://www.commonwl.org/v1.0/Workflow.html#CommandOutputBinding
func (tool *Tool) outputEval(output *cwl.Output, fileArray []*File) (err error) {
	tool.Task.infof("begin output eval for output param %v", output.ID)
	// copy InputsVM to get inputs context
	vm := tool.InputsVM.Copy()

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
	tool.Task.Outputs[output.ID] = evalResult

	tool.Task.infof("end output eval for output param %v", output.ID)
	return nil
}
