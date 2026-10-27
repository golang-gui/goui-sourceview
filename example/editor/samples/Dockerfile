# syntax=docker/dockerfile:1
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/server ./cmd/server

FROM debian:bookworm-slim
ARG APP_USER=editor
ENV APP_PORT=8080
COPY --from=build /out/server /usr/local/bin/server
RUN <<'SETUP'
printf '%s\n' 'Preparing the editor service'
mkdir -p /var/lib/editor
SETUP
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
