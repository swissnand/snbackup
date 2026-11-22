VERSION ?= 1.1.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
PUBKEY  ?=
PKG     := github.com/swissnand/snbackup/internal
LDFLAGS := -s -w -X $(PKG)/version.Version=$(VERSION) -X $(PKG)/version.Commit=$(COMMIT) -X $(PKG)/version.Date=$(shell date -u +%Y-%m-%d) -X $(PKG)/license.PublicKey=$(PUBKEY)

.PHONY: build test dist deb clean
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o snbackup ./cmd/snbackup

test:
	go vet ./...
	GOOS=windows go vet ./...
	go test -race ./...

dist: clean
	mkdir -p dist
	for t in linux/amd64 linux/arm64 windows/amd64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$os-$$arch/snbackup$$ext ./cmd/snbackup; \
	  cp LICENSE README.md dist/$$os-$$arch/; \
	done
	cp packaging/windows/*.ps1 dist/windows-amd64/
	cd dist/linux-amd64 && tar czf ../snbackup-linux-amd64.tar.gz *
	cd dist/linux-arm64 && tar czf ../snbackup-linux-arm64.tar.gz *
	cd dist/windows-amd64 && zip -qr ../snbackup-windows-amd64.zip .
	VERSION=$(VERSION) ARCH=amd64 packaging/linux/build-deb.sh
	VERSION=$(VERSION) ARCH=arm64 packaging/linux/build-deb.sh
	cp dist/snbackup_$(VERSION)_amd64.deb dist/snbackup_amd64.deb
	cp dist/snbackup_$(VERSION)_arm64.deb dist/snbackup_arm64.deb
	cd dist && sha256sum *.tar.gz *.zip *.deb > SHA256SUMS

clean:
	rm -rf dist snbackup
