FROM golang:alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN go get github.com/yuin/gopher-lua
COPY main.go .
RUN go build -o trap-proxy main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/trap-proxy .
COPY rules/ /etc/trap/
EXPOSE 80 443
CMD ["./trap-proxy"]
