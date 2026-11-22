# SWISSNAND Backup – Server und Agent in einem Image
#   Server: docker run -p 8550:8550 -v snbackup:/data ghcr.io/swissnand/snbackup
#   Agent:  docker run ... ghcr.io/swissnand/snbackup agent run  (siehe deploy/)
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
ARG VERSION=dev
ARG COMMIT=dev
ARG PUBKEY=
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/swissnand/snbackup/internal/version.Version=${VERSION} -X github.com/swissnand/snbackup/internal/version.Commit=${COMMIT} -X github.com/swissnand/snbackup/internal/license.PublicKey=${PUBKEY}" \
    -o /out/snbackup ./cmd/snbackup

FROM alpine:3.20
# Werkzeuge für Datenbank-, Docker- und rclone-Backups
RUN apk add --no-cache ca-certificates tzdata postgresql16-client mariadb-client rclone docker-cli
COPY --from=build /out/snbackup /usr/local/bin/snbackup
ENV SNB_DATA=/data \
    TZ=Europe/Zurich
VOLUME ["/data"]
EXPOSE 8550
LABEL org.opencontainers.image.title="SWISSNAND Backup" \
      org.opencontainers.image.description="Verschlüsselte, deduplizierte Backups – Server und Agent" \
      org.opencontainers.image.source="https://github.com/swissnand/snbackup" \
      org.opencontainers.image.vendor="SWISSNAND" \
      org.opencontainers.image.licenses="LicenseRef-SWISSNAND-Community"
ENTRYPOINT ["snbackup"]
CMD ["server", "--data", "/data", "--listen", ":8550"]
