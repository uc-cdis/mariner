FROM golang:1.10-alpine as build

RUN apk update && apk add --no-cache git fuse curl python

# install goofys
RUN go get github.com/kahing/goofys

WORKDIR /root/

# install aws cli
RUN curl "https://s3.amazonaws.com/aws-cli/awscli-bundle.zip" -o "awscli-bundle.zip"
RUN unzip awscli-bundle.zip
RUN ./awscli-bundle/install -b ~/bin/aws

# configure aws credentials - passed in as envVars 
# pretty sure with the environment variables set there's no need to `aws configure set` anything
# see mariner-task-sidecar job spec in mariner/k8s.go
# RUN /root/bin/aws configure set aws_access_key_id $AWS_ACCESS_KEY_ID
# RUN /root/bin/aws configure set aws_secret_access_key $AWS_SECRET_ACCESS_KEY

# create dir which will be mount point for bucket - pretty sure this will already exist actually
# see the mounted volume/mount point for container in k8s job spec
RUN mkdir -p /data

# mount bucket:prefix
RUN goofys workflow-engine-garvin:$S3PREFIX /data
