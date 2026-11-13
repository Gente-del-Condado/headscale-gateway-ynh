GATEWAY_JNI_PATH := $(call my-dir)
include $(GATEWAY_JNI_PATH)/../../../third_party/hev-socks5-tunnel/Android.mk
LOCAL_PATH := $(GATEWAY_JNI_PATH)
include $(CLEAR_VARS)
LOCAL_MODULE := gateway-tunnel
LOCAL_SRC_FILES := tunnel.c
LOCAL_C_INCLUDES := $(LOCAL_PATH)/../../../third_party/hev-socks5-tunnel/src
LOCAL_SHARED_LIBRARIES := hev-socks5-tunnel
LOCAL_LDFLAGS += -Wl,-z,max-page-size=16384
include $(BUILD_SHARED_LIBRARY)
