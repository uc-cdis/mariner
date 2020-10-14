package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

// list envVars here - to be read into the routine
const (
	awsCredsEnvVar     = "AWSCREDS"
	s3RegionEnvVar     = "S3_REGION"
	s3BucketNameEnvVar = "S3_BUCKET_NAME"
)

// S3FileManager manages interactions with S3
type S3FileManager struct {
	AWSConfig *aws.Config
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
