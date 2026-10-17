# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags "-s -w" -o /out/controlplane ./cmd/controlplane

# The control plane starts demo containers by calling the docker command line, so the image carries the client.
FROM alpine:3.21
RUN apk add --no-cache docker-cli ca-certificates && adduser -D -u 10001 cp
COPY --from=build /out/controlplane /usr/local/bin/controlplane
# It must reach the Docker socket, which is root-equivalent on the host: run it on a host that only runs demos, and
# never publish the back-office API without sign-in and a TLS proxy in front.
USER cp
EXPOSE 8090
ENTRYPOINT ["controlplane"]
