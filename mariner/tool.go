package mariner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// this file contains some methods/functions for setting up and working with Tools (i.e., commandlinetools and expressiontools)

// initDirReq handles the InitialWorkDirRequirement if specified for this tool
// TODO: support cases where File or dirent is returned from `entry`
// NOTE: this function really needs to be cleaned up/revised
func (engine *K8sEngine) initWorkDirReq(tool *Tool) (err error) {
	tool.Task.infof("begin handle InitialWorkDirRequirement")
	var resFile interface{}
	for _, requirement := range tool.Task.Root.Requirements {
		if requirement.Class == CWLInitialWorkDirRequirement {
			for _, listing := range requirement.Listing {
				// handling the case where `entry` is content (expression or string) to be written to a file
				// and `entryname` is the name of the file to be created
				var contents interface{}
				// `entry` is an expression which may return a string, File or `dirent`
				// NOTE: presently NOT supporting the File or dirent case
				// what's a dirent? good question: https://www.commonwl.org/v1.0/CommandLineTool.html#Dirent

				// logic: exactly one of resultString or resultFile should be returned
				resultText, resultFile, err := tool.resolveExpressions(listing.Entry)
				tool.Task.infof("resultText: %v", resultText)
				tool.Task.infof("resultFile: %v", resultFile)
				switch {
				case err != nil:
					return tool.Task.errorf("failed to resolve expressions in entry: %v; error: %v", listing.Entry, err)
				case resultFile != nil:
					contents = resultFile
				case resultText != "":
					contents = resultText
				default:
					return tool.Task.errorf("entry returned empty value: %v", listing.Entry)
				}

				// `entryName` for sure is a string literal or an expression which evaluates to a string
				// `entryName` is the name of the file to be created
				entryName, _, err := tool.resolveExpressions(listing.EntryName)
				if err != nil {
					return tool.Task.errorf("failed to resolve expressions in entry name: %v; error: %v", listing.EntryName, err)
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
					tool.Task.errorf("feature not supported: entry expression returned a file object")
				} else {

					// #no-fuse

					sess := engine.S3FileManager.newS3Session()
					uploader := s3manager.NewUploader(sess)

					// Q: what about the case of creating directories?
					// guess: this is probably not currently supported
					key := strings.TrimPrefix(engine.localPathToS3Key(entryName), "/")
					tool.S3Input.Paths = append(tool.S3Input.Paths, entryName)

					var b []byte
					switch contents.(type) {
					case string:
						b = []byte(contents.(string))
					case *File:
						b, err = json.Marshal(contents)
						if err != nil {
							return tool.Task.errorf("error marshalling contents to file: %v", err)
						}
						tool.Task.infof("Converted file to json: %v", b)
					}

					result, err := uploader.Upload(&s3manager.UploadInput{
						Bucket: aws.String(engine.S3FileManager.S3BucketName),
						Key:    aws.String(key),
						Body:   bytes.NewReader(b),
					})
					if err != nil {
						return fmt.Errorf("upload to s3 failed: %v", err)
					}
					tool.Task.infof("wrote initdir bytes to s3 object: %v", result.Location)
					fmt.Println("wrote initdir bytes to s3 object:", result.Location)
					// log
				}
			}
		}
	}
	tool.Task.infof("end handle InitialWorkDirRequirement")
	return nil
}
