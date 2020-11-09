# Mariner S3 Sidecar

## this is a placeholder doc 

to remind @Matt to make an actual doc explaining: 

- what does this sidecar container do
- where does it get used by Mariner
- what config/input values does it require
- what are the basic steps
- what are possible future extensions for further functionality

## basic steps

00. load in vars from envVars
0. configure the AWS interface with the creds
1. read 's3://<twd>/_mariner_s3_paths'
2. download those files from s3
3. signal to main to run
4. wait
5. upload output (?) files to s3
6. exit 0

