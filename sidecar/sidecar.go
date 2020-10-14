package main

import "fmt"

func main() {

	/*
		steps:

		00. load in vars from envVars
		0. configure the AWS interface with the creds
		1. read 's3://<twd>/_mariner_s3_paths'
		2. download those files from s3
		3. signal to main to run

		// here!
		4. wait
		5. upload output (?) files to s3
		6. exit 0

	*/

	// 1. read in the target s3 paths
	s3Paths, err := readMarinerS3Paths()
	if err != nil {
		fmt.Println("readMarinerS3Paths failed:", err)
	}

	// 2. download those files to the shared volume
	err = downloadFiles(s3Paths)
	if err != nil {
		fmt.Println("downloadFiles failed:", err)
	}

	// 3. signal main container to run
	err = signalTaskToRun()
	if err != nil {
		fmt.Println("signalTaskToRun failed:", err)
	}

	return
}

// 1. read 's3://<twd>/_mariner_s3_paths'
func readMarinerS3Paths() ([]string, error) {
	return nil, nil
}

// 2. batch download target s3 paths
func downloadFiles(s3Paths []string) error {
	return nil
}

// 3. signal to main container to run
func signalTaskToRun() error {
	return nil
}
