FROM golang:1.12-alpine as build

RUN apk update && apk add --no-cache git curl python py-pip

# install goofys
RUN go get github.com/kahing/goofys

# WORKDIR /root/

# install aws cli
# RUN curl "https://s3.amazonaws.com/aws-cli/awscli-bundle.zip" -o "awscli-bundle.zip"
# RUN unzip awscli-bundle.zip
# RUN ./awscli-bundle/install -b ~/bin/aws
# RUN ./awscli-bundle/install -i /aws

RUN mkdir /awscli && \
    pip install -U -t=/awscli/ awscli

RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
# WORKDIR /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/

FROM alpine
COPY --from=build /go/src/github.com/uc-cdis/mariner/Docker/s3Sidecar/s3sidecarDockerrun.sh /go/bin/goofys /awscli/aws /
RUN apk update && apk add --no-cache fuse jq

ENTRYPOINT [ "/bin/sh", "/s3sidecarDockerrun.sh" ]