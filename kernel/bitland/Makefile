obj-m := bitland_mifs_wmi_iot.o
bitland_mifs_wmi_iot-y := bitland-mifs-wmi.o
KDIR ?= /lib/modules/$(shell uname -r)/build

.PHONY: all clean
all:
	$(MAKE) -C "$(KDIR)" M="$(CURDIR)" CONFIG_DEBUG_INFO_BTF_MODULES= modules
clean:
	$(MAKE) -C "$(KDIR)" M="$(CURDIR)" clean
