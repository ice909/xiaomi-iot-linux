/* SPDX-License-Identifier: GPL-2.0-only */
#ifndef XIAOMI_IOT_FORWARD_H
#define XIAOMI_IOT_FORWARD_H

#include <linux/dmi.h>
#include <linux/kobject.h>

static bool xiaomi_iot_supported(void)
{
	const char *board = dmi_get_system_info(DMI_BOARD_NAME);

	return dmi_match(DMI_SYS_VENDOR, "XIAOMI") && board &&
		(!strcmp(board, "TM2424") || !strcmp(board, "TM2425") ||
		 !strcmp(board, "TM2426"));
}

static void xiaomi_iot_forward(struct device *dev,
			       const struct wmi_buffer *buffer)
{
	const u8 *bytes = buffer->data;
	char packet[48];
	char *env[] = { "XIAOMI_IOT_BRIDGE=1", packet, NULL };

	/* OEM EventDetail is 32 bytes. Never export its remaining contents. */
	if (buffer->length < 32 || bytes[0] != 1)
		return;
	if (!((bytes[1] == 0x2a && bytes[2] >= 1 && bytes[2] <= 3) ||
	      (bytes[1] == 0x2b && bytes[2] >= 1 && bytes[2] <= 6)))
		return;
	snprintf(packet, sizeof(packet), "XIAOMI_IOT_EVENT=%02x%02x%02x",
		 bytes[0], bytes[1], bytes[2]);
	kobject_uevent_env(&dev->kobj, KOBJ_CHANGE, env);
}

/* Capability belongs to the event device, not the separate control device. */
static ssize_t xiaomi_iot_events_show(struct device *dev,
				    struct device_attribute *attr, char *buf)
{
	return sysfs_emit(buf, "1\n");
}
static DEVICE_ATTR_RO(xiaomi_iot_events);
static struct attribute *xiaomi_iot_attrs[] = {
	&dev_attr_xiaomi_iot_events.attr,
	NULL,
};
static const struct attribute_group xiaomi_iot_group = {
	.attrs = xiaomi_iot_attrs,
};

#endif
