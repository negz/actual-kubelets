FROM golang:1.13.15-alpine3.11 AS build

RUN apk update && apk add git && apk add curl

WORKDIR /go/actual-kubelets
COPY . .

RUN go build -o virtual-kubelet ./cmd/virtual-kubelet

FROM alpine:3.11

RUN apk update && apk add ca-certificates
COPY --from=build /go/actual-kubelets/virtual-kubelet /bin/virtual-kubelet
