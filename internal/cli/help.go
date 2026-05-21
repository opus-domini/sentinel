package cli

import "io"

const (
	sectionCommands = "COMMANDS"
	sectionFlags    = "FLAGS"

	flagExecUsage  = "--exec PATH"
	flagScopeUsage = "--scope SCOPE"
)

// helpRow is a name/description pair rendered as one aligned line.
type helpRow struct {
	name string
	desc string
}

// helpSection is a titled block of help rows (e.g. COMMANDS, FLAGS).
type helpSection struct {
	title string
	rows  []helpRow
}

// helpDoc describes a command's help output in the unified gh-style format:
// a one-line summary, a USAGE block, titled sections and an optional footer.
type helpDoc struct {
	summary  string
	usage    []string
	sections []helpSection
	notes    []string
	footer   string
}

// printHelp renders doc in the standard Sentinel help layout. Name columns are
// aligned across every section so the whole document lines up.
func printHelp(w io.Writer, doc helpDoc) {
	width := 0
	for _, section := range doc.sections {
		for _, row := range section.rows {
			if len(row.name) > width {
				width = len(row.name)
			}
		}
	}

	if doc.summary != "" {
		writeln(w, doc.summary)
	}
	if len(doc.usage) > 0 {
		if doc.summary != "" {
			writeln(w, "")
		}
		writeln(w, "USAGE")
		for _, line := range doc.usage {
			writef(w, "  %s\n", line)
		}
	}
	for _, section := range doc.sections {
		writeln(w, "")
		writeln(w, section.title)
		for _, row := range section.rows {
			if row.desc == "" {
				writef(w, "  %s\n", row.name)
				continue
			}
			writef(w, "  %-*s  %s\n", width, row.name, row.desc)
		}
	}
	for _, note := range doc.notes {
		writeln(w, "")
		writeln(w, note)
	}
	if doc.footer != "" {
		writeln(w, "")
		writeln(w, doc.footer)
	}
}

func printRootHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Sentinel command-line interface",
		usage:   []string{"sentinel <command> [flags]"},
		sections: []helpSection{
			{title: "CORE COMMANDS", rows: []helpRow{
				{"daemon", "Start the Sentinel server"},
				{"service", "Manage the local service and autoupdate timer"},
				{"update", "Check and apply binary updates"},
			}},
			{title: "ADDITIONAL COMMANDS", rows: []helpRow{
				{"doctor", "Check the local environment and runtime config"},
				{"completion", "Generate a shell completion script (bash/zsh/fish)"},
			}},
			{title: sectionFlags, rows: []helpRow{
				{"-h, --help", "Show help"},
				{"-v, --version", "Print the version"},
			}},
		},
		footer: `Run "sentinel <command> --help" for details on a command.`,
	})
}

func printDaemonHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Start the Sentinel server using the config file and environment defaults.",
		usage:   []string{"sentinel daemon"},
	})
}

func printServiceHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Manage the Sentinel background service and autoupdate timer.",
		usage:   []string{"sentinel service <command> [flags]"},
		sections: []helpSection{
			{title: sectionCommands, rows: []helpRow{
				{cmdInstall, "Install the service unit and start it"},
				{cmdUninstall, "Stop the service and remove its unit"},
				{cmdStatus, "Show whether the service is installed and running"},
				{"logs", "Stream the service log"},
				{"autoupdate", "Manage the automatic update timer"},
			}},
		},
		footer: `Run "sentinel service <command> --help" for details on a command.`,
	})
}

func printServiceInstallHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Install the Sentinel service unit and start it.",
		usage:   []string{"sentinel service install [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{flagExecUsage, "Binary path for the service unit (default: current executable)"},
				{"--enable", "Enable the service at startup (default: true)"},
				{"--start", "Start the service immediately (default: true)"},
			}},
		},
	})
}

func printServiceUninstallHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Stop the Sentinel service and remove its unit file.",
		usage:   []string{"sentinel service uninstall [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{"--disable", "Disable the service from auto-start (default: true)"},
				{"--stop", "Stop the running service (default: true)"},
				{"--remove-unit", "Remove the managed unit file (default: true)"},
				{"--purge", "Also remove the autoupdate timer, shell completion and binary"},
			}},
		},
		notes: []string{"--purge leaves user data in ~/.sentinel intact."},
	})
}

func printServiceStatusHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Show whether the Sentinel service is installed, enabled and active.",
		usage:   []string{"sentinel service status"},
	})
}

func printServiceLogsHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Stream the Sentinel service log (journalctl on Linux, plist logs on macOS).",
		usage:   []string{"sentinel service logs [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{"-f, --follow", "Stream new log lines as they arrive"},
				{"-n, --lines N", "Number of past log lines to show (default: 50)"},
			}},
		},
	})
}

func printServiceAutoUpdateHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Manage the Sentinel automatic update timer.",
		usage:   []string{"sentinel service autoupdate <command> [flags]"},
		sections: []helpSection{
			{title: sectionCommands, rows: []helpRow{
				{cmdInstall, "Install the autoupdate timer and start it"},
				{cmdUninstall, "Stop the autoupdate timer and remove its units"},
				{cmdStatus, "Show the autoupdate timer status"},
			}},
		},
		footer: `Run "sentinel service autoupdate <command> --help" for details on a command.`,
	})
}

func printServiceAutoUpdateInstallHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Install the timer that checks for and applies new releases.",
		usage:   []string{"sentinel service autoupdate install [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{flagExecUsage, "Binary path for the updater unit (default: current executable)"},
				{"--enable", "Enable the timer at startup (default: true)"},
				{"--start", "Start the timer immediately (default: true)"},
				{"--service NAME", "Service unit to restart after an update (default: sentinel)"},
				{flagScopeUsage, "Restart manager scope: auto|user|system|launchd (default: auto)"},
				{"--on-calendar WHEN", "Update schedule: daily|hourly|weekly|duration (default: daily)"},
				{"--randomized-delay D", "Randomized delay before updating, systemd only (default: 1h)"},
			}},
		},
	})
}

func printServiceAutoUpdateUninstallHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Stop the autoupdate timer and remove its unit files.",
		usage:   []string{"sentinel service autoupdate uninstall [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{"--disable", "Disable the timer from auto-start (default: true)"},
				{"--stop", "Stop the running timer (default: true)"},
				{"--remove-unit", "Remove the autoupdate unit files (default: true)"},
				{flagScopeUsage, "Target scope: auto|user|system|launchd (default: auto)"},
			}},
		},
	})
}

func printServiceAutoUpdateStatusHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Show the autoupdate timer status.",
		usage:   []string{"sentinel service autoupdate status [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{flagScopeUsage, "Target scope: auto|user|system|launchd (default: auto)"},
			}},
		},
	})
}

func printDoctorHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Check the local environment and runtime configuration.",
		usage:   []string{"sentinel doctor"},
	})
}

func printUpdateHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Check for and apply Sentinel binary updates.",
		usage:   []string{"sentinel update <command> [flags]"},
		sections: []helpSection{
			{title: sectionCommands, rows: []helpRow{
				{"check", "Check whether a newer release is available"},
				{"apply", "Download and install the latest release"},
				{cmdStatus, "Show the last recorded update state"},
			}},
		},
		footer: `Run "sentinel update <command> --help" for details on a command.`,
	})
}

func printUpdateCheckHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Check whether a newer Sentinel release is available.",
		usage:   []string{"sentinel update check [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{"--repo OWNER/NAME", "GitHub repository (default: opus-domini/sentinel)"},
				{"--api URL", "GitHub API base URL override"},
				{"--os OS", "Target operating system (default: current)"},
				{"--arch ARCH", "Target CPU architecture (default: current)"},
			}},
		},
	})
}

func printUpdateApplyHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Download and install the latest Sentinel release.",
		usage:   []string{"sentinel update apply [flags]"},
		sections: []helpSection{
			{title: sectionFlags, rows: []helpRow{
				{"--repo OWNER/NAME", "GitHub repository (default: opus-domini/sentinel)"},
				{"--api URL", "GitHub API base URL override"},
				{"--os OS", "Target operating system (default: current)"},
				{"--arch ARCH", "Target CPU architecture (default: current)"},
				{flagExecUsage, "Binary to replace (default: current executable)"},
				{"--allow-downgrade", "Allow installing an older release (default: false)"},
				{"--allow-unverified", "Allow update when no checksum is available (default: false)"},
				{"--restart", "Restart the managed service after updating (default: false)"},
				{"--service NAME", "Service unit to restart (default: sentinel)"},
				{flagScopeUsage, "Restart scope: auto|user|system|launchd|none"},
			}},
		},
	})
}

func printUpdateStatusHelp(w io.Writer) {
	printHelp(w, helpDoc{
		summary: "Show the last recorded update state.",
		usage:   []string{"sentinel update status"},
	})
}
