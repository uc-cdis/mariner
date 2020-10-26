package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	// environment variables
	awsCredsEnvVar         = "AWSCREDS"
	s3RegionEnvVar         = "S3_REGION"
	s3BucketNameEnvVar     = "S3_BUCKET_NAME"
	userIDEnvVar           = "USER_ID"
	sharedVolumeNameEnvVar = "ENGINE_WORKSPACE"
	taskWorkingDirEnvVar   = "TOOL_WORKING_DIR"

	// setting a max so as to prevent the error of having too many files being open at once
	// need to investigate how high we can set this bound without running into problems
	// for now, conservatively setting the bound to 32
	maxConcurrent = 32

	// resides in the task's working dir in s3
	// contains list of files that need to be downloaded from s3 in order for this task to run
	inputFileListName = "_mariner_s3_input.json"
)

// S3FileManager manages interactions with S3
type S3FileManager struct {
	AWSConfig             *aws.Config
	S3BucketName          string
	InputFileListS3Key    string
	UserID                string
	SharedVolumeMountPath string
	TaskWorkingDir        string
	MaxConcurrent         int
}

type awsCredentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func (fm *S3FileManager) setup() (err error) {
	fm.AWSConfig, err = loadAWSConfig()
	if err != nil {
		return err
	}
	fm.S3BucketName = os.Getenv(s3BucketNameEnvVar)
	fm.UserID = os.Getenv(userIDEnvVar)

	// "/engine-workspace"
	fm.SharedVolumeMountPath = fmt.Sprintf("/%v", sharedVolumeNameEnvVar)

	fm.TaskWorkingDir = os.Getenv(taskWorkingDirEnvVar)

	fm.MaxConcurrent = maxConcurrent

	// "/userID/workflowRuns/runID/taskID/_mariner_s3_input.json"
	fm.InputFileListS3Key = filepath.Join(fm.s3Key(fm.TaskWorkingDir), inputFileListName)

	return nil
}

/*
	converts filepath to the corresponding s3 location
	-> maps the local "task working directory"
	-- to the S3 "task working directory"

	filepaths look like:
	"/engine-workspace/path/to/file"

	s3 keys look like:
	"/userID/path/to/file"

	so, replace "/engine-workspace" with "/userID"
*/
func (fm *S3FileManager) s3Key(path string) string {
	userIDPrefix := fmt.Sprintf("/%v", fm.UserID)
	key := strings.Replace(path, fm.SharedVolumeMountPath, userIDPrefix, 1)
	return key
}

func (fm *S3FileManager) newS3Session() *session.Session {
	return session.Must(session.NewSession(fm.AWSConfig))
}

func loadAWSConfig() (*aws.Config, error) {
	secret := []byte(os.Getenv(awsCredsEnvVar))
	creds := &awsCredentials{}
	err := json.Unmarshal(secret, creds)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling aws secret: %v", err)
	}
	credsConfig := credentials.NewStaticCredentials(creds.ID, creds.Secret, "")
	awsConfig := &aws.Config{
		Region:      aws.String(os.Getenv(s3RegionEnvVar)),
		Credentials: credsConfig,
	}
	return awsConfig, nil
}
