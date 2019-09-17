FROM golang:1.13-alpine as build

RUN apk update && apk add --no-cache git fuse curl python jq

# install goofys
RUN go get github.com/kahing/goofys

# install gen3fuse
RUN go get github.com/uc-cdis/gen3-fuse

WORKDIR /root/

# install aws cli
RUN curl "https://s3.amazonaws.com/aws-cli/awscli-bundle.zip" -o "awscli-bundle.zip"
RUN unzip awscli-bundle.zip
RUN ./awscli-bundle/install -b ~/bin/aws

RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
WORKDIR /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/

# NOTE: work from a scratch image
