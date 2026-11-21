#!/bin/sh
# Builds dist/snbackup_<version>_<arch>.deb from dist/linux-<arch>/snbackup
set -eu
VERSION=${VERSION:-1.0.0}
ARCH=${ARCH:-amd64}
ROOT=$(mktemp -d)
mkdir -p "$ROOT/DEBIAN" "$ROOT/usr/bin" "$ROOT/lib/systemd/system" "$ROOT/usr/share/doc/snbackup"
install -m 0755 "dist/linux-$ARCH/snbackup" "$ROOT/usr/bin/snbackup"
install -m 0644 packaging/linux/snbackup.service "$ROOT/lib/systemd/system/snbackup.service"
install -m 0644 LICENSE "$ROOT/usr/share/doc/snbackup/copyright"
cat > "$ROOT/DEBIAN/control" <<CTL
Package: snbackup
Version: $VERSION
Section: admin
Priority: optional
Architecture: $ARCH
Maintainer: SWISSNAND <https://tools.swissnand.com>
Homepage: https://github.com/swissnand/snbackup
Suggests: postgresql-client, default-mysql-client, rclone
Description: SWISSNAND Backup – verschlüsselte, deduplizierte Backups
 Backup-Server mit Web-Konsole und CLI: Dateien, Datenbanken, Docker,
 Spiegelung, Archive und 3-2-1-Kopien auf Disk, NAS, S3, WebDAV und rclone.
 Kostenlos für nicht-kommerzielle Nutzung, kommerziell lizenzpflichtig.
CTL
install -m 0755 packaging/linux/postinst "$ROOT/DEBIAN/postinst"
install -m 0755 packaging/linux/prerm "$ROOT/DEBIAN/prerm"
dpkg-deb --root-owner-group --build "$ROOT" "dist/snbackup_${VERSION}_${ARCH}.deb"
rm -rf "$ROOT"
