package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

		---

		3. signal to main to run
		4. wait
		5. upload output files to s3 (?)
		6. exit 0

	*/

	fm := &S3FileManager{}
	fm.setup()

	// 1. read in the target s3 paths
	// #okay
	taskS3Input, err := fm.fetchTaskS3InputList()
	if err != nil {
		fmt.Println("readMarinerS3Paths failed:", err)
	}

	// 2. download those files to the shared volume
	// #fixme - no batch download please, thanks - Thursday
	err = fm.downloadInputFiles(taskS3Input)
	if err != nil {
		fmt.Println("downloadFiles failed:", err)
	}

	// 3. signal main container to run
	// #okay
	err = fm.signalTaskToRun()
	if err != nil {
		fmt.Println("signalTaskToRun failed:", err)
	}

	// 4. wait for main container to finish
	// #okay
	err = fm.waitForTaskToFinish()
	if err != nil {
		fmt.Println("waitForTaskToFinish failed:", err)
	}

	// 5. upload output files to s3 (which files, how to decide exactly? - floating issue)
	// #todo - Thursday
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
// NOTE: currently using the batch download, which actually is a bad idea
// reason it's a bad idea: opens opportunity for there to be too many open files at once
// as it stands, as many paths that there are,
// that many files could potentially be open at once,
// and if that number is very larger (larger than the OS max)
// the program will crash
//
// solution: concurrently download individual files,
// open and close each file individually
// cap total number allowed goroutines to something reasonable like 16 or __
//
// HERE - Thursday
func (fm *S3FileManager) downloadInputFiles(taskS3Input *TaskS3Input) error {
	sess := fm.newS3Session()
	svc := s3manager.NewDownloader(sess)

	/*
		paths look like:
		"/engine-workspace/path/to/file"

		for downloading, need to map that to the actual s3 key:
		"/userID/path/to/file"

		so, replace "/engine-workspace" with "/userID"
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

		// replace "/engine-workspace" with "/userID"
		userIDPrefix := fmt.Sprintf("/%v", fm.UserID)
		key = strings.Replace(path, fm.SharedVolumeMountPath, userIDPrefix, 1)

		obj = s3manager.BatchDownloadObject{
			Object: &s3.GetObjectInput{
				Bucket: aws.String(fm.S3BucketName),
				Key:    aws.String(key),
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
// fixme - WHY is it that the sidecar passes the task command to the main container?
// ------> WHY doesn't the engine simply give the task container its command directly?
// ------> early design decision, probably doesn't make sense any more, should fix it
func (fm *S3FileManager) signalTaskToRun() error {
	cmd := os.Getenv("TOOL_COMMAND")
	pathToTaskCommand := filepath.Join(fm.TaskWorkingDir, "run.sh")

	f, err := os.Open(pathToTaskCommand)
	defer f.Close()
	if err != nil {
		return err
	}
	f.WriteString(cmd)

	return nil
}

// 4. wait for main container to finish
// not sure if this fn should actually return an error or not
func (fm *S3FileManager) waitForTaskToFinish() error {
	time.Sleep(10 * time.Second)

	var err error
	doneFlag := filepath.Join(fm.TaskWorkingDir, "done")
	taskDone := false
	for !taskDone {
		_, err = os.Stat(doneFlag)
		switch {
		case err == nil:
			// done flag (file) exists
			taskDone = true
		case os.IsNotExist(err):
			// done flag doesn't exist
		default:
			// unexpected error
			fmt.Println("unexpected error checking for doneFlag:", err)
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

// 5. upload output to s3
func (fm *S3FileManager) uploadOutputFiles() (err error) {
	// collect paths of all files in the task working directory
	paths := []string{}
	err = filepath.Walk(fm.TaskWorkingDir, func(path string, info os.FileInfo, err error) error {
		paths = append(paths, path)
		return nil
	})

	// upload files to the task working directory location in S3
	sess := fm.newS3Session()

	/*
		"Once the Uploader instance is created
		you can call Upload concurrently
		from multiple goroutines safely."
			- aws sdk-for-go docs
	*/
	uploader := s3manager.NewUploader(sess)

	var f *os.File
	var result *s3manager.UploadOutput
	var wg sync.WaitGroup

	guard := make(chan struct{}, fm.MaxConcurrent)
	for _, p := range paths {
		// blocks if guard channel is already full to capacity
		// proceeds as soon as there is an open slot in the channel
		guard <- struct{}{}

		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			// open file for reading
			f, err = os.Open(path)
			if err != nil {
				fmt.Println("failed to open file:", err)
				return
			}

			// upload the file contents
			result, err = uploader.Upload(&s3manager.UploadInput{
				Bucket: aws.String(fm.S3BucketName),
				Key:    aws.String("REPLACEME"), // fix
				Body:   f,
			})
			if err != nil {
				fmt.Println("failed to upload file:", err)
				return
			}

			// close the file - very important
			if err = f.Close(); err != nil {
				fmt.Println("failed to close file:", err)
				return
			}

			fmt.Println("file uploaded to location:", result.Location)

			// release this spot in the guard channel
			// so the next go routine can run
			<-guard
		}(p)
	}
	wg.Wait()
	return nil
}
