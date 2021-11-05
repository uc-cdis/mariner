FROM quay.io/cdis/golang:1.17-bullseye as build-deps

ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
ENV GORACE="history_size=7"

WORKDIR $GOPATH/src/github.com/uc-cdis/mariner/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN GITCOMMIT=`git rev-parse HEAD` \
    GITVERSION=`git describe --always --tags` \
    && go build -race \
    -ldflags="-X 'github.com/uc-cdis/mariner/version.GitCommit=${GITCOMMIT}' -X 'github.com/uc-cdis/mariner/version.GitVersion=${GITVERSION}'" \
    -o /mariner

# FROM scratch
# COPY --from=build-deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# COPY --from=build-deps /mariner /mariner
CMD ["/mariner", "run", "$RUN_ID"]
