FROM golang:1.10-alpine as build

# NOTE: this dockerfile is ready to test

RUN apk update && apk add --no-cache git fuse curl python

# install goofys
RUN go get github.com/kahing/goofys

WORKDIR /root/

# install aws cli
RUN curl "https://s3.amazonaws.com/aws-cli/awscli-bundle.zip" -o "awscli-bundle.zip"
RUN unzip awscli-bundle.zip
RUN ./awscli-bundle/install -b ~/bin/aws

# configure aws credentials - aws creds are a k8s secret, will be exposed as files in a volume
# located at `/awsusercreds/awsusercreds.json` -> see mariner-engine k8s job spec
# see: https://kubernetes.io/docs/concepts/configuration/secret/ "consuming secret values from volumes"
RUN /root/bin/aws configure set aws_access_key_id $(cat /awsusercreds/awsusercreds.json/id)
RUN /root/bin/aws configure set aws_secret_access_key $(cat /awsusercreds/awsusercreds.json/secret)

# create dir which will be mount point for bucket - pretty sure this will already exist actually
# see the mounted volume/mount point for container in k8s job spec
RUN mkdir -p /data

# mount bucket:/prefix/
# NOTE: in mariner-engine job spec, need to load container with these environment variables
RUN goofys workflow-engine-garvin:/$WORKFLOW_USER_ID/$WORKFLOW_ID/ /data
