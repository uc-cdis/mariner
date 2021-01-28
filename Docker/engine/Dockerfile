FROM quay.io/cdis/golang:1.15-buster as build

COPY . $GOPATH/src/github.com/uc-cdis/mariner/
WORKDIR $GOPATH/src/github.com/uc-cdis/mariner/

RUN go get -d -v
RUN go build -ldflags "-linkmode external -extldflags -static" -o /mariner

FROM scratch
COPY --from=build /mariner /

ENTRYPOINT ["/mariner", "run", "$RUN_ID"]
