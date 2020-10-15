package main

import (
	"encoding/json"
	"fmt"
	"os"

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
)

// S3FileManager manages interactions with S3
type S3FileManager struct {
	AWSConfig             *aws.Config
	S3BucketName          string
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
	return nil
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
		Region:      aws.String(s3RegionEnvVar),
		Credentials: credsConfig,
	}
	return awsConfig, nil
}
