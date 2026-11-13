/* Adaptador propio. HEV conserva su licencia MIT en android/third_party. */
#include <jni.h>
#include <unistd.h>
#include "hev-main.h"

JNIEXPORT jint JNICALL JNI_OnLoad(JavaVM *vm, void *reserved) { return JNI_VERSION_1_6; }

JNIEXPORT jint JNICALL Java_org_headscalegateway_android_GatewayVpnService_runTunnel(JNIEnv *env, jclass cls, jstring config, jint fd) {
    const char *path = (*env)->GetStringUTFChars(env, config, NULL);
    if (!path) return -1;
    int owned = dup(fd);
    int result = owned < 0 ? -1 : hev_socks5_tunnel_main(path, owned);
    /* HEV cierra el descriptor recibido; el proceso auxiliar recoge los fallos de inicio. */
    (*env)->ReleaseStringUTFChars(env, config, path);
    return result;
}
JNIEXPORT void JNICALL Java_org_headscalegateway_android_GatewayVpnService_stopTunnel(JNIEnv *env, jclass cls) {
    hev_socks5_tunnel_quit();
}
