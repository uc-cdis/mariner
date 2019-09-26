FROM golang:1.12-alpine as build

# Install SSL certificates
RUN apk update && apk add --no-cache git ca-certificates gcc musl-dev

# Build static mariner binary
RUN mkdir -p /go/src/github.com/uc-cdis/mariner
WORKDIR /go/src/github.com/uc-cdis/mariner
ADD . .

# here using feat/k8s branch for testing deployment
# RUN mkdir -p /go/src/github.com/uc-cdis
# WORKDIR /go/src/github.com/uc-cdis
# RUN git clone -b feat/k8s --single-branch https://github.com/uc-cdis/mariner/
# WORKDIR /go/src/github.com/uc-cdis/mariner

RUN go build -ldflags "-linkmode external -extldflags -static" -o /mariner

# Set up small scratch image, and copy necessary things over
# FROM scratch
# COPY --from=build /mariner /mariner

ENTRYPOINT ["/mariner", "listen"]
