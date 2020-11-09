FROM golang:1.15 as build
ADD . /go/tile38-cluster-manager
RUN make --directory /go/tile38-cluster-manager build

FROM alpine:latest as cert
RUN apk update && apk add ca-certificates

FROM scratch
COPY --from=cert /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/tile38-cluster-manager/pkg/tile38-cluster-manager /tile38-cluster-manager
ENTRYPOINT ["/tile38-cluster-manager"]