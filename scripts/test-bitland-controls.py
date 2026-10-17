#!/usr/bin/env python3
"""Compile actual power/profile handlers against deterministic firmware responses."""
from pathlib import Path
import os, subprocess, tempfile
root=Path(__file__).resolve().parents[1]
stubs=r'''
#include <assert.h>
#include <errno.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
typedef uint8_t u8; typedef uint16_t u16; typedef uint32_t u32;
struct device { void *data; };
struct wmi_device { struct device dev; };
struct wmi_buffer { size_t length; void *data; };
struct bitland_mifs_wmi_data { struct wmi_device *wdev; int lock; };
struct bitland_mifs_input { u8 reserved1,operation,reserved2,function,payload[28]; };
struct bitland_mifs_output { u8 reserved1,operation,reserved2,function,data[28]; };
enum platform_profile_option { PLATFORM_PROFILE_LOW_POWER, PLATFORM_PROFILE_BALANCED, PLATFORM_PROFILE_PERFORMANCE };
#define WMI_METHOD_GET 250
#define WMI_METHOD_SET 251
#define DMI_SYS_VENDOR 0
#define DMI_BOARD_NAME 1
#define DMI_BIOS_VERSION 2
static const char *dmi[] = { "XIAOMI", "TM2425", "RMAPT6B0P0909" };
static bool dmi_match(int id,const char *s){return !strcmp(dmi[id],s);}
static bool xiaomi_iot_supported(void){return true;}
typedef void *acpi_handle;
#define ACPI_SUCCESS(x) ((x)==0)
static int acpi_get_handle(void *a,const char *b,acpi_handle *h){(void)a;(void)b;*h=(void *)1;return 0;}
static int acpi_evaluate_integer(acpi_handle h,const char *b,void *c,unsigned long long *v){(void)h;(void)b;(void)c;*v=15;return 0;}
struct attribute {int mode;};
struct device_attribute {struct attribute attr;};
struct attribute_group {struct attribute **attrs;};
#define DEVICE_ATTR(name,mode,show,store) struct device_attribute dev_attr_##name = {{mode}}
#define guard(kind) (void)
#define CAP_SYS_ADMIN 1
static bool admin=true;
static bool capable(int c){(void)c;return admin;}
static void *dev_get_drvdata(struct device *d){return d->data;}
#define sysfs_emit sprintf
#define kfree free
static u16 get_unaligned_le16(const void *p){const u8 *b=p;return b[0]|(b[1]<<8);}
static u32 get_unaligned_le32(const void *p){const u8 *b=p;return b[0]|(b[1]<<8)|(b[2]<<16)|((u32)b[3]<<24);}
static int supplied=1,calls,sets,capability,fail,malformed;
static u8 raw_mode=9,power_state=1;
static int power_supply_is_system_supplied(void){return supplied;}
static int bitland_mifs_wmi_call(struct bitland_mifs_wmi_data *data,const struct bitland_mifs_input *in,struct bitland_mifs_output *out){
 (void)data; calls++; if(fail)return -EIO;
 memset(out,0,sizeof(*out));out->operation=0x80;
 if(in->operation==WMI_METHOD_GET && in->function==8){out->function=8;out->data[0]=raw_mode;}
 else if(in->operation==WMI_METHOD_GET && in->function==16){assert(in->payload[0]==3);out->function=16;out->data[0]=3;out->data[2]=capability;}
 else if(in->operation==WMI_METHOD_SET && in->function==8){sets++;raw_mode=in->payload[0]; /* SET leaves function zero. */}
 else assert(false);
 if(malformed)out->operation=0xe0;
 return 0;
}
static size_t reply_length=32;
static int wmidev_invoke_method(struct wmi_device *d,int instance,int method,const struct wmi_buffer *in,struct wmi_buffer *out,size_t minimum){
 (void)d;calls++;assert(instance==0&&method==1&&in->length==32);
 const u8 *b=in->data;assert(b[0]==0&&b[2]==0&&b[3]==0x0c&&b[4]==3&&b[5]==0);
 if(fail)return -EIO;
 if(reply_length<minimum)return -EPROTO; /* WMI core's minimum-length contract. */
 u8 *reply=calloc(1,reply_length);assert(reply);reply[1]=malformed?0xe0:0x80;
 if(b[1]==0xfb){assert(b[6]<=1);sets++;power_state=b[6];}
 else assert(b[1]==0xfa);
 if(reply_length>=8){reply[3]=0x0c;reply[4]=3;reply[6]=power_state;}
 out->data=reply;out->length=reply_length;return 0;
}
'''
main=r'''
int main(void){
 struct wmi_device w={0};struct bitland_mifs_wmi_data data={.wdev=&w};w.dev.data=&data;
 assert(xiaomi_tm2425_profiles());dmi[1]="TM2424";assert(!xiaomi_tm2425_profiles());dmi[1]="TM2425";
 dmi[2]="new-bios";assert(!xiaomi_tm2425_profiles());dmi[2]="RMAPT6B0P0909";
 assert(xiaomi_power_supported());
 u8 mode,power;enum platform_profile_option profile;
 for(int i=0;i<256;i++){
  raw_mode=i;int ret=xiaomi_profile_read(&data,&mode);
  bool supported=i==2||i==3||i==4||i==9||i==10;
  assert((ret==0)==supported);
  if(supported){assert(mode==i);assert(xiaomi_profile_decode(mode,&profile)==0);
   assert(profile==(i==2||i==10?PLATFORM_PROFILE_LOW_POWER:i==9?PLATFORM_PROFILE_BALANCED:PLATFORM_PROFILE_PERFORMANCE));}
 }
 capability=0;supplied=1;sets=0;assert(xiaomi_profile_write(&data,4)==0&&sets==1&&raw_mode==4);
 sets=0;capability=1;assert(xiaomi_profile_write(&data,4)==-EOPNOTSUPP&&sets==0);
 capability=2;assert(xiaomi_profile_write(&data,4)==-EPROTO&&sets==0);
 supplied=0;capability=0;assert(xiaomi_profile_write(&data,3)==-EOPNOTSUPP&&sets==0);
 assert(xiaomi_profile_write(&data,9)==0&&raw_mode==9);
 assert(xiaomi_profile_write(&data,10)==0&&raw_mode==10);
 sets=0;for(int i=5;i<=8;i++)assert(xiaomi_profile_write(&data,i)==-EINVAL);assert(sets==0);
 supplied=1;malformed=1;assert(xiaomi_profile_read(&data,&mode)==-EPROTO);assert(xiaomi_profile_write(&data,4)==-EPROTO);
 malformed=0;fail=1;assert(xiaomi_profile_read(&data,&mode)==-EIO);fail=0;
 char buf[8];sets=0;
 assert(iot_power_show(&w.dev,NULL,buf)==2&&!strcmp(buf,"1\n"));assert(sets==0);
 assert(iot_power_store(&w.dev,NULL,"0\n",2)==2&&power_state==0);
 assert(iot_power_store(&w.dev,NULL,"1",1)==1&&power_state==1);
 for(int i=0;i<2;i++){reply_length=i?8:32;assert(xiaomi_power_call(&data,false,0,&power)==0&&power==1);}
 for(int i=0;i<8;i++){reply_length=i;assert(xiaomi_power_call(&data,false,0,&power)==-EPROTO);}
 reply_length=2;assert(xiaomi_power_call(&data,true,1,&power)==0);
 reply_length=32;malformed=1;assert(xiaomi_power_call(&data,false,0,&power)==-EPROTO);malformed=0;
 int old=sets;admin=false;assert(iot_power_store(&w.dev,NULL,"0",1)==-EPERM);assert(sets==old);admin=true;
 const char *bad[]={"","2","01","1 ","0\n\n","-1"};
 for(size_t i=0;i<sizeof(bad)/sizeof(*bad);i++)assert(iot_power_store(&w.dev,NULL,bad[i],strlen(bad[i]))==-EINVAL);
 assert(sets==old);puts("Bitland controls: TM2425 profile protocol, AC guard, raw modes and power validation passed");
}
'''
with tempfile.TemporaryDirectory(prefix='bitland-controls-') as tmp:
 p=Path(tmp);(p/'linux').mkdir();(p/'linux/capability.h').write_text('/* host stubs */\n')
 (p/'test.c').write_text(stubs+'\n#include "xiaomi-profiles.h"\n#include "xiaomi-power.h"\n'+main)
 subprocess.run([os.environ.get('CC','cc'),'-std=gnu11','-Wall','-Wextra','-Werror','-Wno-unused-const-variable','-Wno-unused-parameter','-I',str(p),'-I',str(root/'kernel/bitland'),str(p/'test.c'),'-o',str(p/'test')],check=True)
 subprocess.run([str(p/'test')],check=True)
