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

#### Below this line, need to be passed as arguments from the k8s deployment spec - can't reference envVars in the image

# configure aws credentials - awsusercreds.json passed in as AWSCREDS envVar
# RUN /root/bin/aws configure set aws_access_key_id $(echo $AWSCREDS | jq .id)
# RUN /root/bin/aws configure set aws_secret_access_key $(echo $AWSCREDS | jq .secret)

# mount bucket:prefix
# RUN goofys workflow-engine-garvin:$S3PREFIX /data
