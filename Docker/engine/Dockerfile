FROM golang:1.12-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .

RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/mariner

FROM scratch

# using alpine because need /bin/sh or bash, and copying /bin/sh over to scratch doesn't seem to work
# FROM alpine

COPY --from=build /go/src/github.com/uc-cdis/mariner/bin/mariner /
COPY --from=build /bin/sh /bin/
