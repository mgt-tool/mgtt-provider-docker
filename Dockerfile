# Multi-stage build for mgtt-provider-docker.
# The provider binary shells out to the `docker` CLI, so the runtime image
# must include it. `docker:cli` is the minimal official image that ships
# just the client — no daemon, no buildx, ~30MB.
#
# NOTE: The container needs access to a docker daemon at runtime. When this
# image is used via `mgtt provider install --image`, the operator must pass
# the socket through (e.g. `-v /var/run/docker.sock:/var/run/docker.sock`).
# mgtt's image runner does not yet forward env or mounts; see the upstream
# mgtt issue tracking `ImageRunner` env/volume pass-through.

FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/provider .

FROM docker:cli
COPY --from=build /out/provider /usr/local/bin/provider
COPY provider.yaml /provider.yaml
ENTRYPOINT ["/usr/local/bin/provider"]
