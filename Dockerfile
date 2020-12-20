FROM golang:1.15 as build
WORKDIR /void/ircbot
COPY . .
RUN go mod vendor && \
        CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o /irc .

FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /irc /irc
LABEL org.opencontainers.image.source https://github.com/void-linux/ircbot
ENTRYPOINT ["/irc"]
