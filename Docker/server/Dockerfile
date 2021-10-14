FROM quay.io/cdis/golang:1.17-bullseye as build-deps

RUN apt-get update \
    && apt-get install --no-install-recommends -y jq bash postgresql \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR $GOPATH/src/github.com/uc-cdis/mariner/

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

# RUN go build -o /mariner
RUN go build -o bin/mariner

CMD ["bin/mariner", "listen"]

# FROM scratch
# COPY --from=build-deps /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# COPY --from=build-deps /mariner /mariner
# CMD ["/mariner", "listen"]
