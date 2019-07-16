FROM golang:1.12-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static mariner binary - from master branch - uncomment for prod
# RUN mkdir -p /go/src/github.com/uc-cdis/mariner
# WORKDIR /go/src/github.com/uc-cdis/mariner
# ADD . .

# here using feat/k8s branch for testing deployment
RUN mkdir -p /go/src/github.com/uc-cdis
WORKDIR /go/src/github.com/uc-cdis
RUN git clone -b feat/k8s --single-branch https://github.com/uc-cdis/mariner/
WORKDIR /go/src/github.com/uc-cdis/mariner

RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/mariner

# Set up small scratch image, and copy necessary things over
# FROM scratch

# using alpine because need /bin/sh or bash, and copying /bin/sh over to scratch doesn't seem to work
FROM alpine

COPY --from=build /go/src/github.com/uc-cdis/mariner/bin/mariner /mariner

# commenting out this entrypoint because the engine container needs to wait for the engine-sidecar to mount the bucket and write request.json to /data/request.json
# see mariner/k8s.go the container spec for mariner-engine job
# ENTRYPOINT ["/mariner", "run", ""$S3PREFIX"]
