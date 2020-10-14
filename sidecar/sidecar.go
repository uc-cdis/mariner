package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func main() {

	/*
		steps:

		00. load in vars from envVars
		0. configure the AWS interface with the creds
		1. read 's3://<twd>/_mariner_s3_paths'
		2. download those files from s3
		3. signal to main to run
		4. wait
		5. upload output files to s3 (?)
		6. exit 0

	*/

	fm := &S3FileManager{}
	fm.setup()

	// 1. read in the target s3 paths
	taskS3Input, err := fm.fetchTaskS3InputList()
	if err != nil {
		fmt.Println("readMarinerS3Paths failed:", err)
	}

	// 2. download those files to the shared volume
	err = fm.downloadInputFiles(taskS3Input)
	if err != nil {
		fmt.Println("downloadFiles failed:", err)
	}

	// 3. signal main container to run
	err = fm.signalTaskToRun()
	if err != nil {
		fmt.Println("signalTaskToRun failed:", err)
	}

	// 4. wait for main container to finish
	err = fm.waitForTaskToFinish()
	if err != nil {
		fmt.Println("waitForTaskToFinish failed:", err)
	}

	// 5. upload output files to s3 (which files, how to decide exactly? - floating issue)
	err = fm.uploadOutputFiles()
	if err != nil {
		fmt.Println("uploadOutputFiles failed:", err)
	}

	// 6. exit
	return
}

// TaskS3Input ..
type TaskS3Input struct {
	Paths []string `json:"paths"`
}

// 1. read 's3://<twd>/_mariner_task_s3_input.json'
func (fm *S3FileManager) fetchTaskS3InputList() (*TaskS3Input, error) {
	sess := fm.newS3Session()

	// Create a downloader with the session and default options
	downloader := s3manager.NewDownloader(sess)

	// Create a buffer to write the S3 Object contents to.
	// see: https://stackoverflow.com/questions/41645377/golang-s3-download-to-buffer-using-s3manager-downloader
	buf := &aws.WriteAtBuffer{}

	objKey := "" // fixme -> '/userID/workflowRuns/runID/taskID/_mariner_task_s3_input.json'

	// Write the contents of S3 Object to the buffer
	s3Obj := &s3.GetObjectInput{
		Bucket: aws.String(fm.S3BucketName),
		Key:    aws.String(objKey),
	}
	_, err := downloader.Download(buf, s3Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to download file, %v", err)
	}

	b := buf.Bytes()
	taskS3Input := &TaskS3Input{}
	err = json.Unmarshal(b, taskS3Input)
	if err != nil {
		return nil, fmt.Errorf("error unmarhsalling TaskS3Input: %v", err)
	}
	return taskS3Input, nil
}

/*
	~ Path representations/handling for user-data ~

	s3: 			   "/userID/path/to/file"
	inputs.json: 	      "USER/path/to/file"
	mariner: 		"/engine-workspace/path/to/file"

	user-data bucket gets mounted at the 'userID' prefix to dir /engine-workspace/

	so the mapping that happens in this path processing step is this:
	"USER/path/to/file" -> "/engine-workspace/path/to/file"
*/

// 2. batch download target s3 paths
func (fm *S3FileManager) downloadInputFiles(taskS3Input *TaskS3Input) error {
	// what you want: https://docs.aws.amazon.com/sdk-for-go/api/service/s3/s3manager/#BatchDownloadIterator
	// -------> func (Downloader) DownloadWithIterator
	// for downloading batches of files

	sess := fm.newS3Session()
	svc := s3manager.NewDownloader(sess)

	// s3 path: 's3://workflow-engine-garvin/userID/workflowRuns/runID/taskID/'
	// '/engine-workspace/'

	/*
		paths look like:
		"/engine-workspace/path/to/file"

		for downloading, need to map that to the actual s3 key:
		"/userID/path/to/file"

		so, replace 'engine-workspace' with '<userID>'
	*/

	var obj s3manager.BatchDownloadObject
	var key string
	objects := []s3manager.BatchDownloadObject{}
	for _, path := range taskS3Input.Paths {

		destFile, err := os.Open(path)
		if err != nil {
			// probably fatal
			return err
		}

		key = strings.Replace(path, "engine-workspace", fm.UserID, 1)

		obj = s3manager.BatchDownloadObject{
			Object: &s3.GetObjectInput{
				Bucket: aws.String(fm.S3BucketName),
				Key:    aws.String(key), // fixme
			},
			Writer: destFile,
		}

		objects = append(objects, obj)
	}

	iter := &s3manager.DownloadObjectsIterator{Objects: objects}
	err := svc.DownloadWithIterator(aws.BackgroundContext(), iter)
	if err != nil {
		return err
	}

	return nil
}

// 3. signal to main container to run
func (fm *S3FileManager) signalTaskToRun() error {
	return nil
}

// 4. wait for main container to finish
func (fm *S3FileManager) waitForTaskToFinish() error {
	return nil
}

// 5. upload output to s3
func (fm *S3FileManager) uploadOutputFiles() error {
	return nil
}
