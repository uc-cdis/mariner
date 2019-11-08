FROM golang:1.12-alpine as build

RUN apk update && apk add --no-cache git 

# install goofys
RUN go get github.com/kahing/goofys

# get bash script from mariner repo
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .

# work from small python:alpine image
FROM python:alpine
COPY --from=build /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/s3sidecarDockerrun.sh /go/bin/goofys /
RUN apk update && apk add --no-cache fuse jq ca-certificates
RUN pip install awscli

ENTRYPOINT [ "/bin/sh", "/s3sidecarDockerrun.sh" ]