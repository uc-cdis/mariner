FROM quay.io/cdis/golang:1.15-buster as builder

COPY . $GOPATH/src/github.com/uc-cdis/mariner/
WORKDIR $GOPATH/src/github.com/uc-cdis/mariner/

RUN go get -d -v
RUN go build -ldflags "-linkmode external -extldflags -static" -o /mariner

FROM scratch
COPY --from=builder /mariner /
# Copy CA certificates to prevent x509: certificate signed by unknown authority errors
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/mariner", "run", "$RUN_ID"]
