FROM golang:1.12-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .
RUN go build -ldflags "-linkmode external -extldflags -static" -o /mariner

FROM scratch
COPY --from=build /mariner /

ENTRYPOINT ["/mariner", "listen"]