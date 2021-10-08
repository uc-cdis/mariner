package mariner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"
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
				tool.Task.infof("listing: %+v", listing)

				// logic: exactly one of resultString or resultFile should be returned
				if len(listing.Entry) == 0 {
					// Here we have the case for an expression/string and not a dirent
					tool.Task.infof("listing entry len 0: %v", listing.Entry)
					tool.Task.infof("listing Location: %v", listing.Location)
					tool.Task.infof("listing Location type: %T", listing.Location)
					if strings.HasPrefix(listing.Location, "$(") {
						tool.Task.infof("listing Location has JS expression: %v", listing.Location)
						output, err := tool.evalExpression(listing.Location)
						switch {
						case err != nil:
							log.Errorf("failed to evaluate expression: %v; error: %v", listing.Location, err)
							return tool.Task.errorf("failed to evaluate expression: %v; error: %v", listing.Location, err)
						case isFile(output):
							tool.Task.infof("Is a file: %v", output)
						case isArrayOfFile(output):
							tool.Task.infof("Is an array of files: %v", output)
							out, err := tool.processFileList(output)
							if err != nil {
								return tool.Task.errorf("Couldn't parse file list: %v", err)
							}
							tool.Task.infof("parsed file list: %v", out)
						default:
							log.Errorf("Not a filetype: %v", output)
							return tool.Task.errorf("Not a filetype: %T; error: %v", output, err)
						}
						tool.Task.infof("output: %v", output)
						tool.Task.infof("output Type: %T", output)
					}
					continue
				}

				resultText, resultFile, err := tool.resolveExpressions(listing.Entry)
				switch {
				case err != nil:
					log.Errorf("failed to resolve expressions in entry: %v; error: %v", listing.Entry, err)
					return tool.Task.errorf("failed to resolve expressions in entry: %v; error: %v", listing.Entry, err)
				case resultFile != nil:
					contents = resultFile
				case resultText != "":
					contents = resultText
				default:
					log.Errorf("entry returned empty value: %v", listing.Entry)
					return tool.Task.errorf("entry returned empty value: %v", listing.Entry)
				}

				// `entryName` for sure is a string literal or an expression which evaluates to a string
				// `entryName` is the name of the file to be created
				entryName, _, err := tool.resolveExpressions(listing.EntryName)
				if err != nil {
					log.Errorf("failed to resolve expressions in entry name: %v; error: %v", listing.EntryName, err)
					return tool.Task.errorf("failed to resolve expressions in entry name: %v; error: %v", listing.EntryName, err)
				}

				/*
					NOTE: I think we DO support the file case - though maybe not the dirent case
						Cases:
						1. `entry` returned a file object - file object stored as an interface{} in `resFile` (NOT SUPPORTED)
						2. `entry` did not return a file object - then returned value is in `contents` and must be written to a new file with filename stored in `entryName` (supported)
				*/

				// pretty sure this conditional is dated/unnecessary
				tool.Task.infof("resFile: %v", resFile)
				if resFile != nil {
					// "If the value is an expression that evaluates to a File object,
					// this indicates the referenced file should be added to the designated output directory prior to executing the tool."
					// NOTE: the "designated output directory" is just the directory corresponding to the Tool
					// not sure what the purpose/meaning/use of this feature is - pretty sure all i/o for Tools gets handled already
					// presently not supporting this case - will implement this feature once I find an example to work with
					log.Errorf("feature not supported: entry expression returned a file object")
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
							log.Errorf("error marshalling contents to file: %v", err)
							return tool.Task.errorf("error marshalling contents to file: %v", err)
						}
					}

					workDirPath := engine.S3FileManager.s3Key(tool.WorkingDir, engine.UserID)
					key = filepath.Join(workDirPath, key)

					_, err := uploader.Upload(&s3manager.UploadInput{
						Bucket: aws.String(engine.S3FileManager.S3BucketName),
						Key:    aws.String(key),
						Body:   bytes.NewReader(b),
					})

					if err != nil {
						log.Errorf("upload to s3 failed: %v", err)
						return fmt.Errorf("upload to s3 failed: %v", err)
					}
					log.Infof("init working directory request recieved")
					engine.IsInitWorkDir = "true"
				}
			}
		}
	}
	log.Infof("end handle InitialWorkDirRequirement")
	return nil
}
