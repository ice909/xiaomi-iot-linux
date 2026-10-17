/* SPDX-License-Identifier: GPL-2.0-only */
#ifndef XIAOMI_IOT_POWER_H
#define XIAOMI_IOT_POWER_H

#include <linux/capability.h>

/* Uses the control device's WMI lock, also shared by all Bitland functions. */
static int xiaomi_power_call(struct bitland_mifs_wmi_data *data, bool set,
			    u8 value, u8 *power)
{
	u8 frame[32] = { 0x00, 0xfa, 0x00, 0x0c, 0x03, 0x00 };
	const u8 prefix[] = { 0x00, 0x80, 0x00, 0x0c, 0x03, 0x00 };
	struct wmi_buffer input = { .length = sizeof(frame), .data = frame };
	struct wmi_buffer output = {};
	u8 *reply;
	int ret;

	if (value > 1)
		return -EINVAL;
	if (set) {
		frame[1] = 0xfb;
		frame[6] = value;
	}
	ret = wmidev_invoke_method(data->wdev, 0, 1, &input, &output, set ? 2 : 8);
	if (ret)
		return ret;
	reply = output.data;
	ret = -EPROTO;
	if (set) {
		if (reply[0] == 0x00 && reply[1] == 0x80)
			ret = 0;
	} else if (output.length <= sizeof(frame) && !memcmp(reply, prefix, sizeof(prefix)) &&
		   reply[6] <= 1 && reply[7] == 0) {
		*power = reply[6];
		ret = 0;
	}
	kfree(output.data);
	return ret;
}

static ssize_t iot_power_show(struct device *dev, struct device_attribute *attr, char *buf)
{
	struct bitland_mifs_wmi_data *data = dev_get_drvdata(dev);
	u8 power;
	int ret;

	guard(mutex)(&data->lock);
	ret = xiaomi_power_call(data, false, 0, &power);
	return ret ? ret : sysfs_emit(buf, "%u\n", power);
}

static ssize_t iot_power_store(struct device *dev, struct device_attribute *attr,
			       const char *buf, size_t count)
{
	struct bitland_mifs_wmi_data *data = dev_get_drvdata(dev);
	u8 power, value;
	int ret;

	if (!capable(CAP_SYS_ADMIN))
		return -EPERM;
	if (!((count == 1 || (count == 2 && buf[1] == '\n')) &&
	      (buf[0] == '0' || buf[0] == '1')))
		return -EINVAL;
	value = buf[0] - '0';
	guard(mutex)(&data->lock);
	ret = xiaomi_power_call(data, true, value, &power);
	if (!ret) {
		ret = xiaomi_power_call(data, false, 0, &power);
		if (!ret && power != value)
			ret = -EIO;
	}
	return ret ? ret : (ssize_t)count;
}

static DEVICE_ATTR(iot_power, 0600, iot_power_show, iot_power_store);
static struct attribute *xiaomi_power_attrs[] = { &dev_attr_iot_power.attr, NULL };
static const struct attribute_group xiaomi_power_group = { .attrs = xiaomi_power_attrs };

static bool xiaomi_power_supported(void)
{
	acpi_handle handle;
	unsigned long long status;

	return xiaomi_iot_supported() &&
		ACPI_SUCCESS(acpi_get_handle(NULL, "\\_SB.PC00.IOTD", &handle)) &&
		ACPI_SUCCESS(acpi_evaluate_integer(handle, "_STA", NULL, &status)) && status == 0x0f;
}
#endif
