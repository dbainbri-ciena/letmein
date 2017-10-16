FROM golang:1.8-alpine as builder
MAINTAINER Ciena Corporation

WORKDIR /go
ADD . /go/src/github.com/ciena/letmein
RUN go build -o /build/entry-point github.com/ciena/letmein

FROM alpine:3.6
MAINTAINER Ciena Corporation
COPY --from=builder /build/entry-point /service/entry-point
COPY rule.tmpl /var/templates/create.tmpl
WORKDIR /service
ENTRYPOINT ["/service/entry-point"]
