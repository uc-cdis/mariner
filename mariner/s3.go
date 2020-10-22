package mariner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

const (
	// environment variables
	awsCredsEnvVar         = "AWSCREDS"
	userIDEnvVar           = "USER_ID"
	sharedVolumeNameEnvVar = "ENGINE_WORKSPACE"

	// setting a max so as to prevent the error of having too many files being open at once
	// need to investigate how high we can set this bound without running into problems
	// for now, conservatively setting the bound to 32
	maxConcurrent = 32

	// resides in the task's working dir in s3
	// contains list of files that need to be downloaded from s3 in order for this task to run
	inputFileListName = "_mariner_s3_input.json"
)

// S3FileManager ..
type S3FileManager struct {
	AWSConfig     *aws.Config
	S3BucketName  string
	MaxConcurrent int
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
		Region:      aws.String(Config.Storage.S3.Region),
		Credentials: credsConfig,
	}
	return awsConfig, nil
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
	fm.S3BucketName = Config.Storage.S3.Name
	fm.MaxConcurrent = maxConcurrent
	return nil
}

func (fm *S3FileManager) newS3Session() *session.Session {
	return session.Must(session.NewSession(fm.AWSConfig))
}

/*
	converts filepath to the corresponding s3 location
	-> maps the local "task working directory"
	-- to the S3 "task working directory"

	filepaths look like:
	"/engine-workspace/path/to/file"

	s3 keys look like:
	"/userID/path/to/file"

	so, replace "engine-workspace" with "userID"
*/
func (fm *S3FileManager) s3Key(path string, userID string) string {
	key := strings.Replace(path, engineWorkspaceVolumeName, userID, 1)
	return key
}
