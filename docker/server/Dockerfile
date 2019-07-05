FROM golang:1.10-alpine as build

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

ENTRYPOINT ["/gen3cwl listen"]
