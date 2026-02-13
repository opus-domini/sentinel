package recovery

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func currentBootID(ctx context.Context) string {
	if raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v
		}
	}

	sysctlCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(sysctlCtx, "sysctl", "-n", "kern.boottime").Output()
	if err == nil {
		v := strings.TrimSpace(string(out))
		v = strings.Join(strings.Fields(v), " ")
		if v != "" {
			return v
		}
	}

	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, time.Now().UTC().Unix()/300)
}

func shellQuoteSingle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	var buf bytes.Buffer
	buf.WriteByte('\'')
	for _, r := range value {
		if r == '\'' {
			buf.WriteString(`'\''`)
			continue
		}
		buf.WriteRune(r)
	}
	buf.WriteByte('\'')
	return buf.String()
}
