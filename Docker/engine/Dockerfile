FROM quay.io/cdis/golang:1.12-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
RUN go build -ldflags "-linkmode external -extldflags -static" -o bin/mariner

# avoid max pull rate limit errors
# FROM alpine
# COPY --from=build /go/src/github.com/uc-cdis/mariner/bin/mariner /
RUN mv /go/src/github.com/uc-cdis/mariner/bin/mariner /

RUN apk update && apk add --no-cache ca-certificates

ENTRYPOINT [ "/mariner", "run", "$RUN_ID" ]
