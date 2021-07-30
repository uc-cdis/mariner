# Mariner S3 Sidecar

## this is a placeholder doc

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

## what does the side car container do

the side car container is meant to be an alternative form of file download compared to gen3fuse. It is much more performant compared to gen3fuse. It reads a list of files from _mariner_s3_input.json and download the files from s3 into a local directory. Big caveat is that _mariner_s3_input.json is only able to read files from USER s3 buckets and cannot read from /commons-data as of this moment.

## What input values does it require

In your request.json file for a workflow, input a list of files like the example below

"input": {
    "gds_files": [
      {
        "class": "File",
        "location": "USER/1KG_ALL.autosomes.phase3_shapeit2_mvncall_integrated_v5a.20130502.genotypes.gds"
      }
    ]
  },

USER will map to the personal s3 bucket of the user who runs the workflow.
