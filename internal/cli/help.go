package cli

import "github.com/spf13/cobra"

// Help command groups, in display order — a gh-CLI-style layout.
const (
	groupCore  = "core"
	groupExtra = "additional"
)

// usageTemplate is a gh-CLI-style usage template: uppercase section headers
// (USAGE, FLAGS, …) and grouped commands. It mirrors cobra's default template
// structure verbatim — only the section labels are changed — so the whitespace
// handling stays exactly as cobra renders it.
const usageTemplate = `USAGE{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

ALIASES
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

EXAMPLES
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

COMMANDS{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

ADDITIONAL COMMANDS{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

FLAGS
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

GLOBAL FLAGS
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

HELP TOPICS{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Run "{{.CommandPath}} [command] --help" for details on a command.{{end}}
`

// applyHelpStyle wires the gh-style command groups and usage template onto the
// root command. Call it before addGrouped.
func applyHelpStyle(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupCore, Title: "CORE COMMANDS"},
		&cobra.Group{ID: groupExtra, Title: "ADDITIONAL COMMANDS"},
	)
	root.SetHelpCommandGroupID(groupExtra)
	root.SetCompletionCommandGroupID(groupExtra)
	root.SetUsageTemplate(usageTemplate)
}

// addGrouped assigns each command to a help group and registers it on root.
func addGrouped(root *cobra.Command, groupID string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupID
		root.AddCommand(c)
	}
}
