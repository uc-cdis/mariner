FROM golang:1.10-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/mariner

# Set up small scratch image, and copy necessary things over
FROM scratch

COPY --from=build /go/src/github.com/uc-cdis/mariner/bin/mariner /mariner

# commenting out this entrypoint because the engine container needs to wait for the engine-sidecar to mount the bucket and write request.json to /data/request.json
# see mariner/k8s.go the container spec for mariner-engine job
# ENTRYPOINT ["/mariner run $S3PREFIX"]
