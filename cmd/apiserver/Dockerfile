FROM golang:1.20-alpine as builder
ARG OS
ARG ARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

ARG API_GROUP_NAME=backup.cv.io

# Build
RUN CGO_ENABLED=0 \
    GOOS=${OS} \
    GOARCH=${ARCH} \
    go build \
        -o apiserver \
        cmd/apiserver/main.go

FROM alpine:3

WORKDIR /
COPY --from=builder /workspace/apiserver /

ENTRYPOINT ["/apiserver"]
