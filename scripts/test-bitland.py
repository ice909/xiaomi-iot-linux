#!/usr/bin/env python3
"""Run real event and PM callbacks with fake kernel sinks, never hardware."""
import hashlib
import os
from pathlib import Path
import re
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
source = root / "kernel/bitland"
old = (source / "upstream/bitland-mifs-wmi.c").read_text()
new = (source / "bitland-mifs-wmi.c").read_text()
assert hashlib.sha256(old.encode()).hexdigest() == "9284e7c762689273de8cc98f371deeb6ec3b1ebf127d7194a7631fd965f8889a"

def declaration(kind, name):
    return re.search(rf"{kind} {name} \{{.*?\n\}}[^;]*;", old, re.S).group()

def callback(text):
    start = text.index("static void bitland_mifs_wmi_notify(")
    end = text.index("\nstatic const struct wmi_device_id", start)
    return text[start:end]

stubs = r'''
#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>
typedef uint8_t u8;
typedef uint16_t u16;
#define __packed __attribute__((packed))
struct kobject { int unused; };
struct device { struct kobject kobj; void *data; };
struct wmi_device { struct device dev; };
struct wmi_buffer { size_t length; void *data; };
struct attribute { int unused; };
struct device_attribute { struct attribute attr; };
struct attribute_group { struct attribute **attrs; };
#define DEVICE_ATTR_RO(name) struct device_attribute dev_attr_##name
#define sysfs_emit(buf, fmt) sprintf(buf, fmt)
#define KOBJ_CHANGE 2
#define DMI_BOARD_NAME 1
#define DMI_SYS_VENDOR 2
static const char *vendor = "XIAOMI", *board = "TM2425";
static const char *dmi_get_system_info(int key) { return board; }
static bool dmi_match(int key, const char *value) { return !strcmp(vendor, value); }
static int exports;
static char exported[48];
static int kobject_uevent_env(struct kobject *k, int action, char **env) {
    assert(action == KOBJ_CHANGE);
    assert(!strcmp(env[0], "XIAOMI_IOT_BRIDGE=1"));
    assert(env[2] == NULL);
    assert(strlen(env[1]) == strlen("XIAOMI_IOT_EVENT=") + 6);
    strcpy(exported, env[1]); exports++;
    return -5; /* A failed userspace notification must not alter original handling. */
}
#define dev_dbg(...) ((void)0)
#define dev_warn(...) ((void)0)
#define guard(kind) (void)
struct bitland_mifs_wmi_data { bool xiaomi_iot_events; int lock; void *input_dev; };
static void *dev_get_drvdata(struct device *dev) { return dev->data; }
static int bitland_notifier_list;
struct observation { int notifications, action, brightness, channel, speed, keys, key; };
static struct observation seen;
'''

sinks = r'''
static int blocking_notifier_call_chain(int *head, int action, void *value) {
    seen.notifications++; seen.action = action;
    if (action == BITLAND_NOTIFY_KBD_BRIGHTNESS) seen.brightness = *(u8 *)value;
    if (action == BITLAND_NOTIFY_HWMON) {
        struct bitland_fan_notify_data *fan = value;
        seen.channel = fan->channel; seen.speed = fan->speed;
    }
    return 0;
}
static bool sparse_keymap_report_event(void *input, int key, int value, bool release) {
    assert(value == 1 && release); seen.keys++; seen.key = key; return true;
}
'''

main = r'''
int main(void) {
    struct bitland_mifs_wmi_data data = { .xiaomi_iot_events = true };
    struct wmi_device dev = { .dev.data = &data };
    u8 bytes[32];
    struct wmi_buffer buffer = { .data = bytes };
    assert(xiaomi_iot_supported());
    board = "TM2424"; assert(xiaomi_iot_supported());
    board = "TM2426"; assert(xiaomi_iot_supported());
    board = "unknown"; assert(!xiaomi_iot_supported());
    board = NULL; assert(!xiaomi_iot_supported());
    board = "TM2425"; vendor = "other"; assert(!xiaomi_iot_supported());
    vendor = "XIAOMI";
    char cap[8]; assert(xiaomi_iot_events_show(&dev.dev, NULL, cap) == 2);
    assert(!strcmp(cap, "1\n"));
    for (int enabled = 0; enabled <= 1; enabled++)
    for (int length = 8; length <= 32; length += 24)
    for (int type = 0; type <= 2; type++)
    for (int id = 0; id <= 255; id++)
    for (int value = 0; value <= 255; value++) {
        memset(bytes, 0xa5, sizeof(bytes));
        bytes[0] = type; bytes[1] = id; bytes[2] = value;
        buffer.length = length; data.xiaomi_iot_events = enabled;
        memset(&seen, 0, sizeof(seen));
        original_notify(&dev, &buffer);
        struct observation expected = seen;
        memset(&seen, 0, sizeof(seen)); exports = 0;
        bitland_mifs_wmi_notify(&dev, &buffer);
        assert(!memcmp(&expected, &seen, sizeof(seen)));
        bool iot = enabled && length >= 32 && type == 1 &&
            ((id == 0x2a && value >= 1 && value <= 3) ||
             (id == 0x2b && value >= 1 && value <= 6));
        assert(exports == (iot ? 1 : 0));
        if (iot) {
            char packet[48];
            snprintf(packet, sizeof(packet), "XIAOMI_IOT_EVENT=%02x%02x%02x", type, id, value);
            assert(!strcmp(exported, packet));
        }
    }
    bytes[0] = 1; bytes[1] = 0x2a; bytes[2] = 1;
    for (int length = 0; length < 32; length++) {
        buffer.length = length; exports = 0;
        xiaomi_iot_forward(&dev.dev, &buffer); assert(exports == 0);
    }
    puts("Bitland callbacks: 786432 old/new comparisons passed; IoT filtering and capability passed");
    return 0;
}
'''

pm_stubs = r'''
#include <assert.h>
#include <stdbool.h>
#include <stddef.h>
#include <errno.h>
#include <stdio.h>
enum platform_profile_option { LOW_POWER, BALANCED, PERFORMANCE };
struct device { void *data; };
struct bitland_mifs_wmi_data {
    struct device *pp_dev;
    enum platform_profile_option saved_profile;
    bool saved_profile_valid;
    bool tm2425_profiles;
    unsigned char saved_mode;
};
static void *dev_get_drvdata(struct device *dev) { return dev->data; }
static int get_error, set_error, gets, sets, warnings;
static enum platform_profile_option firmware_profile, restored_profile;
static struct device *expected_pp_dev;
#define dev_dbg(...) ((void)0)
#define dev_warn(...) (warnings++)
static int laptop_profile_get(struct device *dev, enum platform_profile_option *profile) {
    assert(dev == expected_pp_dev);
    gets++;
    if (!get_error) *profile = firmware_profile;
    return get_error;
}
static int laptop_profile_set(struct device *dev, enum platform_profile_option profile) {
    assert(dev == expected_pp_dev);
    sets++;
    restored_profile = profile;
    return set_error;
}
'''

pm_stubs += r'''
static int xiaomi_profile_read(struct bitland_mifs_wmi_data *data, unsigned char *mode) { (void)data; *mode = 3; return get_error; }
static int xiaomi_profile_decode(unsigned short mode, enum platform_profile_option *profile) { (void)mode; *profile = PERFORMANCE; return 0; }
static int xiaomi_profile_write(struct bitland_mifs_wmi_data *data, unsigned char mode) { (void)data; assert(mode == 3); sets++; return set_error; }
'''

pm_main = r'''
int main(void) {
    struct device pp_dev = {0};
    struct bitland_mifs_wmi_data data = { .pp_dev = &pp_dev };
    struct device control = { .data = &data };
    struct bitland_mifs_wmi_data event_data = {0};
    struct device event = { .data = &event_data };
    expected_pp_dev = &pp_dev;

    /* Event interfaces have no profile; neither callback may access one. */
    assert(bitland_mifs_wmi_suspend(&event) == 0);
    assert(bitland_mifs_wmi_resume(&event) == 0);
    assert(gets == 0 && sets == 0 && warnings == 0);
    assert(bitland_mifs_wmi_resume(&control) == 0);
    assert(sets == 0);

    /* A successfully saved profile is restored exactly once. */
    firmware_profile = PERFORMANCE;
    assert(bitland_mifs_wmi_suspend(&control) == 0);
    assert(data.saved_profile_valid && gets == 1);
    assert(bitland_mifs_wmi_resume(&control) == 0);
    assert(sets == 1 && restored_profile == PERFORMANCE);
    assert(!data.saved_profile_valid);
    assert(bitland_mifs_wmi_resume(&control) == 0 && sets == 1);

    /* Failed reads must allow sleep without restoring old or default modes. */
    const int errors[] = { -EINVAL, -EIO, -EOPNOTSUPP };
    for (size_t i = 0; i < sizeof(errors) / sizeof(errors[0]); i++) {
        get_error = 0;
        assert(bitland_mifs_wmi_suspend(&control) == 0);
        assert(data.saved_profile_valid);
        get_error = errors[i];
        int old_sets = sets, old_warnings = warnings;
        assert(bitland_mifs_wmi_suspend(&control) == 0);
        assert(!data.saved_profile_valid && warnings == old_warnings + 1);
        assert(bitland_mifs_wmi_resume(&control) == 0 && sets == old_sets);
    }

    /* Restore failure (e.g. unplugged power) must not fail resume or retry. */
    get_error = 0;
    for (size_t i = 0; i < sizeof(errors) / sizeof(errors[0]); i++) {
        firmware_profile = BALANCED;
        assert(bitland_mifs_wmi_suspend(&control) == 0);
        int old_sets = sets, old_warnings = warnings;
        set_error = errors[i];
        assert(bitland_mifs_wmi_resume(&control) == 0);
        assert(sets == old_sets + 1 && restored_profile == BALANCED);
        assert(warnings == old_warnings + 1 && !data.saved_profile_valid);
        assert(bitland_mifs_wmi_resume(&control) == 0 && sets == old_sets + 1);
    }
    set_error = 0;
    data.tm2425_profiles = true;
    assert(bitland_mifs_wmi_suspend(&control) == 0);
    assert(data.saved_mode == 3);
    assert(bitland_mifs_wmi_resume(&control) == 0);
    data.tm2425_profiles = false;
    firmware_profile = LOW_POWER;
    assert(bitland_mifs_wmi_suspend(&control) == 0);
    assert(bitland_mifs_wmi_resume(&control) == 0);
    assert(restored_profile == LOW_POWER);
    puts("Bitland PM callbacks: event skip, valid restore, query/restore errors and stale state passed");
}
'''

with tempfile.TemporaryDirectory(prefix="xiaomi-bitland-test-") as temp:
    work = Path(temp)
    (work / "bitland-mifs-wmi.c").write_text(old)
    subprocess.run(["patch", "--batch", "-p1", "-i", str(source / "xiaomi-iot.patch")], cwd=work, check=True, stdout=subprocess.DEVNULL)
    assert (work / "bitland-mifs-wmi.c").read_text() == new, "bundled patch/source mismatch"
    for name in ("dmi", "kobject"):
        (work / "linux").mkdir(exist_ok=True)
        (work / "linux" / f"{name}.h").write_text("/* Supplied by host harness. */\n")
    code = stubs
    for kind, name in (("enum", "bitland_mifs_event_id"), ("enum", "bitland_mifs_event_type"),
                       ("enum", "bitland_notifier_actions"), ("struct", "bitland_mifs_event"),
                       ("struct", "bitland_fan_notify_data")):
        code += declaration(kind, name) + "\n"
    code += sinks + '#include "xiaomi-iot.h"\n'
    code += callback(old).replace("bitland_mifs_wmi_notify", "original_notify")
    code += callback(new) + main
    (work / "test.c").write_text(code)
    subprocess.run([os.environ.get("CC", "cc"), "-std=gnu11", "-O2", "-Wall", "-Wextra", "-Werror",
                    "-Wno-unused-parameter", "-Wno-unused-const-variable", "-I", str(work),
                    "-I", str(source), str(work / "test.c"), "-o", str(work / "test")], check=True)
    subprocess.run([str(work / "test")], check=True)
    pm_start = new.index("static int bitland_mifs_wmi_suspend(")
    pm_end = new.index("\nstatic DEFINE_SIMPLE_DEV_PM_OPS", pm_start)
    (work / "pm.c").write_text(pm_stubs + new[pm_start:pm_end] + pm_main)
    subprocess.run([os.environ.get("CC", "cc"), "-std=gnu11", "-O2", "-Wall", "-Wextra", "-Werror",
                    str(work / "pm.c"), "-o", str(work / "pm")], check=True)
    subprocess.run([str(work / "pm")], check=True)
