APP := xiaomi-iot-setup
VERSION := 0.1.0
KVER ?= $(shell uname -r)
KDIR ?= /lib/modules/$(KVER)/build

.PHONY: all build test vet kmod bitland-kmod test-bitland install uninstall dist clean
all: build
build:
	mkdir -p build
	CGO_ENABLED=0 go build -trimpath -tags mygo_noinspector -ldflags='-s -w' -o build/xiaomi-iot ./cmd/xiaomi-iot
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o build/xiaomi-iot-helper ./cmd/xiaomi-iot-helper
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o build/xiaomi-iot-remote ./cmd/xiaomi-iot-remote
test:
	go test ./...
	$(MAKE) test-bitland
	python3 scripts/test-kernel-targets.py
test-bitland:
	python3 scripts/test-bitland.py
	python3 scripts/test-bitland-controls.py
vet:
	go vet ./...
	go tool mygo vet .
kmod: bitland-kmod
bitland-kmod:
	@test -f "$(KDIR)/Makefile" || { echo '缺少目标内核头文件；请指定 KDIR。' >&2; exit 1; }
	./kernel/bitland/check-kernel.sh "$(KVER)"
	rm -rf build/bitland
	mkdir -p build/bitland
	cp kernel/bitland/bitland-mifs-wmi.c kernel/bitland/xiaomi-iot.h kernel/bitland/xiaomi-power.h kernel/bitland/xiaomi-profiles.h kernel/bitland/Makefile build/bitland/
	$(MAKE) -C build/bitland KDIR="$(abspath $(KDIR))"
install:
	./install.sh
uninstall:
	./uninstall.sh
dist: build
	rm -rf dist/$(APP)-$(VERSION)
	mkdir -p dist/$(APP)-$(VERSION)/build
	cp build/xiaomi-iot build/xiaomi-iot-helper build/xiaomi-iot-remote dist/$(APP)-$(VERSION)/build/
	cp -r cmd internal packaging resources kernel docs scripts dist/$(APP)-$(VERSION)/
	cp README.md LICENSE Makefile install.sh uninstall.sh go.mod go.sum dist/$(APP)-$(VERSION)/
	tar -C dist -czf dist/$(APP)-$(VERSION)-linux-$(shell uname -m).tar.gz $(APP)-$(VERSION)
	cd dist && sha256sum $(APP)-$(VERSION)-linux-$(shell uname -m).tar.gz > SHA256SUMS
clean:
	rm -rf build dist
