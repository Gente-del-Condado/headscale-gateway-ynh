package admin

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// getent consulta NSS del anfitrión, incluidos los grupos LDAP de YunoHost.
// No necesita contraseñas, secretos LDAP ni una lista de administradores en caché.
func unixGroupHasUser(parent context.Context, group, user string) bool {
	if !unixGroupPattern.MatchString(group) || !proxyUserPattern.MatchString(user) {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/getent", "group", group)
	command.WaitDelay = 200 * time.Millisecond
	var output boundedGroupOutput
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return false
	}
	return groupRecordHasUser(output.String(), group, user)
}

type boundedGroupOutput struct{ buffer bytes.Buffer }

func (b *boundedGroupOutput) String() string { return b.buffer.String() }

func (b *boundedGroupOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 64<<10 {
		return 0, errors.New("group record too large")
	}
	return b.buffer.Write(p)
}

func groupRecordHasUser(record, group, user string) bool {
	record = strings.TrimSpace(record)
	if strings.ContainsAny(record, "\r\n") {
		return false
	}
	fields := strings.Split(record, ":")
	if len(fields) != 4 || fields[0] != group || fields[2] == "" {
		return false
	}
	for _, member := range strings.Split(fields[3], ",") {
		if member == user && member != "" {
			return true
		}
	}
	return false
}
