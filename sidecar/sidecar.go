package main

import (
	"encoding/json"
	"fmt"
	"os"
	"net/url"
	pathLib "path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"
)

// TaskS3Input ..
type TaskS3Input struct {
        URL         string `json:"url"`            // S3 URL
        Path        string `json:"path"`           // Local path for dl
        InitWorkDir bool   `json:"init_work_dir"`  // is this an initwkdir requirement?
}

func main() {

	fm := &S3FileManager{}
	fm.setup()

	// 1. read in the target s3 paths
	taskS3Input, err := fm.fetchTaskS3InputList()
	if err != nil {
		log.Errorf("readMarinerS3Paths failed:", err)
	}

	// 2. download those files to the shared volume
	err = fm.downloadInputFiles(taskS3Input)
	if err != nil {
		log.Errorf("downloadFiles failed:", err)
	}

	// 3. signal main container to run
	err = fm.signalTaskToRun()
	if err != nil {
		log.Errorf("signalTaskToRun failed:", err)
	}

	// 4. wait for main container to finish
	err = fm.waitForTaskToFinish()
	if err != nil {
		log.Errorf("waitForTaskToFinish failed:", err)
	}

	// 5. upload output files to s3
	err = fm.uploadOutputFiles()
	if err != nil {
		log.Errorf("uploadOutputFiles failed:", err)
	}

}

// 1. read this task's input file list from s3
func (fm *S3FileManager) fetchTaskS3InputList() ([]*TaskS3Input, error) {
	sess := fm.newS3Session()

	// Create a downloader with the session and default options
	downloader := s3manager.NewDownloader(sess)

	// Create a buffer to write the S3 Object contents to.
	// see: https://stackoverflow.com/questions/41645377/golang-s3-download-to-buffer-using-s3manager-downloader
	buf := &aws.WriteAtBuffer{}

	// Write the contents of S3 Object to the buffer
	s3Obj := &s3.GetObjectInput{
		Bucket: aws.String(fm.S3BucketName),
		Key:    aws.String(fm.InputFileListS3Key),
	}

	log.Debugf("here are the input key we are trying to download from s3 %s", fm.InputFileListS3Key)
	_, err := downloader.Download(buf, s3Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to download file, %v", err)
	}

	b := buf.Bytes()
	var taskS3Input []*TaskS3Input
	err = json.Unmarshal(b, &taskS3Input)
	if err != nil {
		return nil, fmt.Errorf("error unmarhsalling TaskS3Input: %v", err)
	}

	return taskS3Input, nil
}

// 2. download this task's input files from s3
func (fm *S3FileManager) downloadInputFiles(taskS3Input []*TaskS3Input) (err error) {

	// note: downloader is safe for concurrent use
	sess := fm.newS3Session()
	downloader := s3manager.NewDownloader(sess)

	var wg sync.WaitGroup
	guard := make(chan struct{}, fm.MaxConcurrent)

	//initWorkDirFiles := strings.Split(os.Getenv("InitWorkDirFiles"), ",")
	//fileMaps := make(map[string]bool)
	//for _, val := range initWorkDirFiles {
	//	fileMaps[val] = true
	//}

	for _, p := range taskS3Input {
		// blocks if guard channel is already full to capacity
		// proceeds as soon as there is an open slot in the channel
		guard <- struct{}{}

		wg.Add(1)
		go func(taskInput *TaskS3Input) {
			defer wg.Done()
			log.Debugf("here is the file we are downloading %+v", taskInput)

			// create necessary dirs
			if err = os.MkdirAll(filepath.Dir(taskInput.Path)), os.ModeDir); err != nil {
				log.Errorf("failed to make dirs: %v\n", err)
			}

			// create/open file for writing
			f, err := os.Create(taskInput.Path)
			if err != nil {
				log.Errorf("failed to open file:", err)
			}
			defer f.Close()

			if taskInput.URL != "" {
				parsed, err := url.Parse(taskInput.URL)
				if err != nil {
					log.Errorf("failed parsing URI: %v; error: %v\n", taskInput.URL, err)
				}
				key := strings.TrimPrefix(parsed.Path, "/")

				log.Debugf("trying to download obj with key: %v", key)

				// write s3 object content into file
				_, err = downloader.Download(f, &s3.GetObjectInput{
					Bucket: aws.String(parsed.Host),
					Key:    aws.String(key),
				})
				if err != nil {
					log.Errorf("failed to download file:", taskInput.URL, err)
				}
			} else {
				path := taskInput.Path
				log.Debugf("trying to download obj with key:", fm.s3Key(path))

				// write s3 object content into file
				_, err = downloader.Download(f, &s3.GetObjectInput{
					Bucket: aws.String(fm.S3BucketName),
					Key:    aws.String(strings.TrimPrefix(fm.s3Key(path), "/")),
				})
				if err != nil {
					log.Errorf("failed to download file:", path, err)
				}
			}

			// If initworkdir, we will symlink
			if taskInput.InitWorkDir {
				log.Infof("InitWorkDir file: %v\n", taskInput.Path)
				newPath := filepath.Join(fm.TaskWorkingDir, pathLib.Base(taskInput.Path))
				err = os.Symlink(taskInput.Path, newPath)
				if err != nil {
					log.Infof("skipping symlink: %v - %v; error: %v\n", taskInput.Path, newPath, err)
				} else {
					log.Infof("created symlink: %v - %v\n", taskInput.Path, newPath)
				}
			}

			// release this spot in the guard channel
			// so the next goroutine can run
			<-guard
		}(p)
	}
	wg.Wait()
	return nil
}

// 3. signal to main container to run
// fixme - WHY is it that the sidecar passes the task command to the main container?
// ------> WHY doesn't the engine simply give the task container its command directly?
// ------> early design decision, probably doesn't make sense any more, should fix it
func (fm *S3FileManager) signalTaskToRun() error {

	// cushion to ensure gen3fuse finishes setting up..
	time.Sleep(7 * time.Second)

	// fixme - make these strings constants
	cmd := os.Getenv("TOOL_COMMAND")

	pathToTaskCommand := filepath.Join(fm.TaskWorkingDir, "run.sh")

	// create necessary dirs
	if err := os.MkdirAll(filepath.Dir(pathToTaskCommand), os.ModeDir); err != nil {
		fmt.Printf("failed to make dirs: %v\n", err)
	}

	f, err := os.Create(pathToTaskCommand)
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

	/*
		context:
		when the task process finishes
		it writes a file called "done" to the task working directory
		as soon as that file exists, we can proceed and upload the task output to s3
	*/

	doneFlag := filepath.Join(fm.TaskWorkingDir, "done")
	taskDone := false
	for !taskDone {
		_, err = os.Stat(doneFlag)
		switch {
		case err == nil:
			// 'done' file exists
			taskDone = true
		case os.IsNotExist(err):
			// 'done' file doesn't exist
		default:
			// unexpected error
			log.Errorf("unexpected error checking for doneFlag:", err)
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

// uploadOutputFiles utilizes a file manager to upload output files for a task.
func (fm *S3FileManager) uploadOutputFiles() (err error) {
	paths := []string{}
	_ = filepath.Walk(fm.TaskWorkingDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			if info.Mode().IsRegular() {
				paths = append(paths, path)
			}
		}
		return nil
	})
	sess := fm.newS3Session()
	uploader := s3manager.NewUploader(sess)
	var result *s3manager.UploadOutput
	var wg sync.WaitGroup
	guard := make(chan struct{}, fm.MaxConcurrent)
	for _, p := range paths {
		guard <- struct{}{}
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			f, err := os.Open(path)
			if err != nil {
				log.Errorf("failed to open file:", path, err)
				return
			}

			result, err = uploader.Upload(&s3manager.UploadInput{
				Bucket: aws.String(fm.S3BucketName),
				Key:    aws.String(strings.TrimPrefix(fm.s3Key(path), "/")),
				Body:   f,
			})
			if err != nil {
				log.Errorf("failed to upload file:", path, err)
				return
			}
			fmt.Println("file uploaded to location:", result.Location)
			if err = f.Close(); err != nil {
				log.Errorf("failed to close file:", err)
			}
			<-guard
		}(p)
	}
	wg.Wait()
	return nil
}
