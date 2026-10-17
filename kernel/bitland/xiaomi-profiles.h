/* SPDX-License-Identifier: GPL-2.0-only */
#ifndef XIAOMI_TM2425_PROFILES_H
#define XIAOMI_TM2425_PROFILES_H

/* Proven against this machine's DSDT + XMCC1806 SSDT; see docs/TM2425-PROFILES.md. */
static bool xiaomi_tm2425_profiles(void)
{
	return dmi_match(DMI_SYS_VENDOR, "XIAOMI") &&
		dmi_match(DMI_BOARD_NAME, "TM2425") &&
		dmi_match(DMI_BIOS_VERSION, "RMAPT6B0P0909");
}

static int xiaomi_profile_decode(u16 mode, enum platform_profile_option *profile)
{
	switch (mode) {
	case 2:
	case 10:
		*profile = PLATFORM_PROFILE_LOW_POWER;
		return 0;
	case 9:
		*profile = PLATFORM_PROFILE_BALANCED;
		return 0;
	case 3:
	case 4:
		*profile = PLATFORM_PROFILE_PERFORMANCE;
		return 0;
	default:
		return -EINVAL;
	}
}

static int xiaomi_profile_read(struct bitland_mifs_wmi_data *data, u8 *mode)
{
	struct bitland_mifs_input input = { .operation = WMI_METHOD_GET, .function = 8 };
	struct bitland_mifs_output result;
	enum platform_profile_option profile;
	u16 raw;
	int ret = bitland_mifs_wmi_call(data, &input, &result);

	if (ret)
		return ret;
	if (result.reserved1 || result.operation != 0x80 || result.reserved2 || result.function != 8)
		return -EPROTO;
	raw = get_unaligned_le16(result.data);
	ret = xiaomi_profile_decode(raw, &profile);
	if (!ret)
		*mode = raw;
	return ret;
}

static int xiaomi_profile_write(struct bitland_mifs_wmi_data *data, u8 mode)
{
	struct bitland_mifs_input input = { .operation = WMI_METHOD_GET, .function = 16 };
	struct bitland_mifs_output result;
	enum platform_profile_option profile;
	u32 capability;
	int ret = xiaomi_profile_decode(mode, &profile);

	if (ret)
		return ret;
	if (profile == PLATFORM_PROFILE_PERFORMANCE) {
		/* Conservative policy using firmware selector 3 (ADPW >= 100). */
		if (power_supply_is_system_supplied() <= 0)
			return -EOPNOTSUPP;
		input.payload[0] = 3;
		ret = bitland_mifs_wmi_call(data, &input, &result);
		if (ret)
			return ret;
		if (result.reserved1 || result.operation != 0x80 || result.reserved2 ||
		    result.function != 16 || get_unaligned_le16(result.data) != 3)
			return -EPROTO;
		capability = get_unaligned_le32(result.data + 2);
		if (capability > 1)
			return -EPROTO;
		if (capability)
			return -EOPNOTSUPP;
	}
	memset(&input, 0, sizeof(input));
	input.operation = WMI_METHOD_SET;
	input.function = 8;
	input.payload[0] = mode;
	ret = bitland_mifs_wmi_call(data, &input, &result);
	if (ret)
		return ret;
	/* SET acknowledges status only: FUTR remains zero on this firmware. */
	if (result.reserved1 || result.operation != 0x80 || result.reserved2 ||
	    (result.function != 0 && result.function != 8))
		return -EPROTO;
	return 0;
}
#endif
