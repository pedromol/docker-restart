FROM golang:alpine as build

WORKDIR /go/src/app

COPY main.go /go/src/app
COPY go.mod /go/src/app
COPY go.sum /go/src/app

RUN apk add git

RUN go mod tidy
RUN CGO_ENABLED=0 go build -ldflags '-extldflags "-static" -w -s' -tags timetzdata ./...

FROM scratch

COPY --from=build /go/src/app/goci /main
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 2223

ENTRYPOINT ["/main"]
