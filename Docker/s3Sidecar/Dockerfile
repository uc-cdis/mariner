FROM quay.io/cdis/golang:1.17-bullseye as build-deps

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR $GOPATH/src/github.com/uc-cdis/mariner/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY ./.git/ ./.git/
COPY ./version/ ./version/
COPY ./sidecar/ ./sidecar/

RUN GITCOMMIT=`git rev-parse HEAD` \
    GITVERSION=`git describe --always --tags` \
    && go build \
    -ldflags="-X 'github.com/uc-cdis/mariner/version.GitCommit=${GITCOMMIT}' -X 'github.com/uc-cdis/mariner/version.GitVersion=${GITVERSION}'" \
    -o /sidecar ./sidecar/

FROM scratch
COPY --from=build-deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build-deps /sidecar /sidecar
CMD ["/sidecar"]
