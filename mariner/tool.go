package mariner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// this file contains some methods/functions for setting up and working with Tools (i.e., commandlinetools and expressiontools)

// initDirReq handles the InitialWorkDirRequirement if specified for this tool
// TODO: support cases where File or dirent is returned from `entry`
// NOTE: this function really needs to be cleaned up/revised
func (tool *Tool) initWorkDir() (err error) {
	tool.Task.Log.Event.info("begin handle InitialWorkDirRequirement")
	var resFile interface{}
	var path string
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == initialWorkDirRequirement {
			for _, listing := range requirement.Listing {
				// handling the case where `entry` is content (expression or string) to be written to a file
				// and `entryname` is the name of the file to be created
				var contents interface{}
				// `entry` is an expression which may return a string, File or `dirent`
				// NOTE: presently NOT supporting the File or dirent case
				// what's a dirent? good question: https://www.commonwl.org/v1.0/CommandLineTool.html#Dirent

				// logic: exactly one of resultString or resultFile should be returned
				resultText, resultFile, err := tool.resolveExpressions(listing.Entry)
				switch {
				case err != nil:
					return tool.Task.Log.Event.errorf("failed to resolve expressions in entry: %v; error: %v", listing.Entry, err)
				case resultFile != nil:
					contents = resultFile
				case resultText != "":
					contents = resultText
				default:
					return tool.Task.Log.Event.errorf("entry returned empty value: %v", listing.Entry)
				}

				// `entryName` for sure is a string literal or an expression which evaluates to a string
				// `entryName` is the name of the file to be created
				entryName, _, err := tool.resolveExpressions(listing.EntryName)
				if err != nil {
					return tool.Task.Log.Event.errorf("failed to resolve expressions in entry name: %v; error: %v", listing.EntryName, err)
				}

				/*
					NOTE: I think we DO support the file case - though maybe not the dirent case
						Cases:
						1. `entry` returned a file object - file object stored as an interface{} in `resFile` (NOT SUPPORTED)
						2. `entry` did not return a file object - then returned value is in `contents` and must be written to a new file with filename stored in `entryName` (supported)
				*/

				// pretty sure this conditional is dated/unnecessary
				if resFile != nil {
					// "If the value is an expression that evaluates to a File object,
					// this indicates the referenced file should be added to the designated output directory prior to executing the tool."
					// NOTE: the "designated output directory" is just the directory corresponding to the Tool
					// not sure what the purpose/meaning/use of this feature is - pretty sure all i/o for Tools gets handled already
					// presently not supporting this case - will implement this feature once I find an example to work with
					tool.Task.Log.Event.error("feature not supported: entry expression returned a file object")
				} else {
					// create tool working dir if it doesn't already exist
					// might be unnecessary to put here if dir already created earlier in processing this tool - need to check
					os.MkdirAll(tool.WorkingDir, os.ModePerm)
					path = filepath.Join(tool.WorkingDir, entryName)

					tool.Task.Log.Event.infof("begin create path: %v", path)
					f, err := os.Create(path)
					if err != nil {
						return tool.Task.Log.Event.errorf("failed to create file: %v; error: %v", path, err)
					}
					tool.Task.Log.Event.infof("end create path: %v", path)

					tool.Task.Log.Event.info("begin write bytes to file")
					var b []byte
					switch contents.(type) {
					case string:
						b = []byte(contents.(string))
					case *File:
						b, err = json.Marshal(contents)
						if err != nil {
							return tool.Task.Log.Event.errorf("error marshalling contents to file: %v", err)
						}
					}
					if _, err = f.Write(b); err != nil {
						return tool.Task.Log.Event.error("failed to write bytes to file")
					}
					tool.Task.Log.Event.info("end write bytes to file")

					f.Close()
				}
			}
		}
	}
	tool.Task.Log.Event.info("end handle InitialWorkDirRequirement")
	return nil
}
