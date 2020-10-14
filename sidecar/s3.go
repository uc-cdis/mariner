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
	awsCredsEnvVar = "AWSCREDS"
	s3RegionEnvVar = "S3_REGION"
)

type awsCredentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

func newS3Session() (*session.Session, error) {
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
	sess := session.Must(session.NewSession(awsConfig))
	return sess, nil
}
