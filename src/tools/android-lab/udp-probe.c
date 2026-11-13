/* Prueba desde el UID de adb shell: el UID de la aplicación excluye el TUN. */
#include <sys/socket.h>
#include <sys/time.h>
#include <netdb.h>
#include <unistd.h>
#include <stdio.h>
#include <string.h>
#include <arpa/inet.h>

static int dns_check(const char *name) {
    unsigned char packet[512] = {0x67, 0x17, 1, 0, 0, 1};
    size_t length = 12;
    const char *label = name;
    while (*label) {
        const char *dot = strchr(label, '.');
        size_t count = dot ? (size_t)(dot - label) : strlen(label);
        if (!count || count > 63 || length + count + 6 > sizeof(packet)) return 2;
        packet[length++] = count; memcpy(packet + length, label, count); length += count;
        if (!dot) break;
        label = dot + 1;
    }
    packet[length++] = 0; packet[length++] = 0; packet[length++] = 1;
    packet[length++] = 0; packet[length++] = 1;
    int fd = socket(AF_INET, SOCK_DGRAM, 0);
    struct timeval timeout = {.tv_sec = 8};
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));
    struct sockaddr_in server = {.sin_family = AF_INET, .sin_port = htons(53)};
    inet_pton(AF_INET, "100.100.100.100", &server.sin_addr);
    if (sendto(fd, packet, length, 0, (struct sockaddr *)&server, sizeof(server)) < 0) {close(fd); return 1;}
    int received = recv(fd, packet, sizeof(packet), 0); close(fd);
    if (received < 12 || packet[0] != 0x67 || packet[1] != 0x17 || (packet[3] & 15) || !(packet[6] || packet[7])) {
        fprintf(stderr, "MagicDNS no devolvió una respuesta A\n"); return 1;
    }
    puts("MagicDNS-Gateway-verificado"); return 0;
}

int main(int argc, char **argv) {
    if (argc == 3 && !strcmp(argv[1], "--dns")) return dns_check(argv[2]);
    if (argc != 2) return 2;
    struct addrinfo hints = {.ai_family = AF_INET, .ai_socktype = SOCK_DGRAM}, *peer;
    int error = getaddrinfo(argv[1], "18083", &hints, &peer);
    if (error) { fprintf(stderr, "DNS: %s\n", gai_strerror(error)); return 1; }
    int fd = socket(peer->ai_family, peer->ai_socktype, peer->ai_protocol);
    struct timeval timeout = {.tv_sec = 8};
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));
    const char *message = "UDP-Gateway-verificado";
    char response[128];
    int sent = sendto(fd, message, strlen(message), 0, peer->ai_addr, peer->ai_addrlen);
    int received = recv(fd, response, sizeof(response), 0);
    close(fd); freeaddrinfo(peer);
    if (sent < 0 || received != strlen(message) || memcmp(response, message, received)) {
        perror("No se recibió el eco UDP"); return 1;
    }
    puts("UDP-Gateway-verificado"); return 0;
}
