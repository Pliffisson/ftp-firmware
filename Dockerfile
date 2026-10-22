FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY tftp-monitor.go .
RUN CGO_ENABLED=0 go build -o /tftp-monitor tftp-monitor.go

FROM alpine:3.21

RUN apk add --no-cache tftp-hpa
COPY --from=builder /tftp-monitor /usr/local/bin/tftp-monitor
COPY start-tftp.sh /usr/local/bin/start-tftp

EXPOSE 69/udp

CMD ["/bin/sh", "/usr/local/bin/start-tftp"]
