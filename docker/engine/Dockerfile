FROM golang:1.10-alpine as build

# NOTE: this is verbatim the dockerfile for the mariner server
# but entrypoint is `/gen3cwl run` instead of `/gen3cwl listen`
# not sure if I should have one dockerfile with some simple conditional logic to pick a particular entrypoint
# or if it's better to have two totally separate dockerfiles
# additionally, need to rename the repo to `mariner`

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static gen3cwl binary
RUN mkdir -p /go/src/github.com/uc-cdis/gen3cwl
WORKDIR /go/src/github.com/uc-cdis/gen3cwl
ADD . .
RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/gen3cwl

# Set up small scratch image, and copy necessary things over
FROM scratch

COPY --from=build /go/src/github.com/uc-cdis/gen3cwl/bin/gen3cwl /gen3cwl

# commenting out the entrypoint because the engine container needs to wait for the engine-sidecar to mount the bucket and write request.json to /data/request.json
# ENTRYPOINT ["/gen3cwl run"]
