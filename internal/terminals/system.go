package terminals

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type SystemTerminal struct {
	ID           string `json:"id"`
	TTY          string `json:"tty"`
	User         string `json:"user"`
	ProcessCount int    `json:"processCount"`
	LeaderPID    int    `json:"leaderPid"`
	Command      string `json:"command"`
	Args         string `json:"args"`
}

type terminalGroup struct {
	tty          string
	user         string
	processCount int
	leaderPID    int
	command      string
	args         string
	score        int
}

type parsedPSTerminalLine struct {
	pid     int
	tty     string
	user    string
	command string
	args    string
}

func ListSystem(ctx context.Context) ([]SystemTerminal, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,tty=,user=,comm=,args=")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return nil, fmt.Errorf("ps failed: %s", stderr)
		}
		return nil, fmt.Errorf("ps failed: %w", err)
	}
	return parsePSOutput(out)
}

func parsePSOutput(out []byte) ([]SystemTerminal, error) {
	groups := make(map[string]*terminalGroup)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line, ok := parsePSTerminalLine(scanner.Text())
		if !ok {
			continue
		}
		updateTerminalGroup(groups, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	outList := make([]SystemTerminal, 0, len(groups))
	for _, group := range groups {
		outList = append(outList, SystemTerminal{
			ID:           group.tty,
			TTY:          group.tty,
			User:         group.user,
			ProcessCount: group.processCount,
			LeaderPID:    group.leaderPID,
			Command:      group.command,
			Args:         group.args,
		})
	}

	sort.Slice(outList, func(i, j int) bool {
		return outList[i].TTY < outList[j].TTY
	})

	return outList, nil
}

func parsePSTerminalLine(raw string) (parsedPSTerminalLine, bool) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return parsedPSTerminalLine{}, false
	}

	fields := strings.Fields(line)
	if len(fields) < 5 {
		return parsedPSTerminalLine{}, false
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid <= 0 {
		return parsedPSTerminalLine{}, false
	}

	tty := fields[2]
	if !isInteractiveTTY(tty) {
		return parsedPSTerminalLine{}, false
	}

	user := fields[3]
	command := fields[4]
	args := command
	if len(fields) > 5 {
		args = strings.Join(fields[5:], " ")
	}

	return parsedPSTerminalLine{
		pid:     pid,
		tty:     tty,
		user:    user,
		command: command,
		args:    args,
	}, true
}

func updateTerminalGroup(groups map[string]*terminalGroup, line parsedPSTerminalLine) {
	group := groups[line.tty]
	if group == nil {
		group = &terminalGroup{
			tty:       line.tty,
			user:      line.user,
			leaderPID: line.pid,
		}
		groups[line.tty] = group
	}

	group.processCount++
	score := commandScore(line.command, line.args)
	isBetterCandidate := group.command == "" || score > group.score || (score == group.score && line.pid < group.leaderPID)
	if !isBetterCandidate {
		return
	}

	group.score = score
	group.command = line.command
	group.args = line.args
	group.leaderPID = line.pid
	if line.user != "" {
		group.user = line.user
	}
}

func isInteractiveTTY(tty string) bool {
	if tty == "" || tty == "?" || tty == "-" {
		return false
	}
	return strings.HasPrefix(tty, "pts/") || strings.HasPrefix(tty, "ttys") || strings.HasPrefix(tty, "tty")
}

type TerminalProcess struct {
	PID     int     `json:"pid"`
	PPID    int     `json:"ppid"`
	User    string  `json:"user"`
	Command string  `json:"command"`
	Args    string  `json:"args"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
}

func ListProcesses(ctx context.Context, tty string) ([]TerminalProcess, error) {
	if !isInteractiveTTY(tty) {
		return nil, fmt.Errorf("invalid tty: %s", tty)
	}

	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,tty=,user=,%cpu=,%mem=,comm=,args=")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return nil, fmt.Errorf("ps failed: %s", stderr)
		}
		return nil, fmt.Errorf("ps failed: %w", err)
	}
	return parseProcessOutput(out, tty)
}

func parseProcessOutput(out []byte, filterTTY string) ([]TerminalProcess, error) {
	var processes []TerminalProcess
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}

		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		tty := fields[2]
		if tty != filterTTY {
			continue
		}

		user := fields[3]
		cpu, _ := strconv.ParseFloat(fields[4], 64)
		mem, _ := strconv.ParseFloat(fields[5], 64)
		command := fields[6]
		args := command
		if len(fields) > 7 {
			args = strings.Join(fields[7:], " ")
		}

		processes = append(processes, TerminalProcess{
			PID:     pid,
			PPID:    ppid,
			User:    user,
			Command: command,
			Args:    args,
			CPU:     cpu,
			Mem:     mem,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(processes, func(i, j int) bool {
		return processes[i].PID < processes[j].PID
	})

	return processes, nil
}

func commandScore(command, args string) int {
	switch command {
	case "zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh", "nu":
		return 100
	case "tmux":
		return 90
	case "ssh":
		return 80
	}

	if strings.Contains(args, "codex") {
		return 85
	}
	if strings.Contains(args, "node") || strings.Contains(args, "python") {
		return 70
	}
	return 50
}
