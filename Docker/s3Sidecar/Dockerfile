FROM golang:1.12-alpine as build

RUN apk update && apk add --no-cache git fuse curl python jq

# install goofys
RUN go get github.com/kahing/goofys

WORKDIR /root/

# install aws cli
RUN curl "https://s3.amazonaws.com/aws-cli/awscli-bundle.zip" -o "awscli-bundle.zip"
RUN unzip awscli-bundle.zip
RUN ./awscli-bundle/install -b ~/bin/aws

# create dir which will be mount point for bucket - pretty sure this will already exist actually
# see the mounted volume/mount point for container in k8s job spec
RUN mkdir -p /data

RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
WORKDIR /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/

# clone mariner repo to get setup bash script
# RUN mkdir -p /go/src/github.com/uc-cdis
# WORKDIR /go/src/github.com/uc-cdis
# RUN git clone -b feat/k8s --single-branch https://github.com/uc-cdis/mariner/
# WORKDIR /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/

# NOTE: work from a scratch image
