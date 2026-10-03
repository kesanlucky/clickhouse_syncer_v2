package ui

import (
	"fmt"
	"strings"
)

// ValidationStep is the display-layer representation of one validation check.
type ValidationStep struct {
	Name    string
	Passed  bool
	Message string
}

// PrintValidationResult renders a grouped, human-readable validation summary.
//
// Steps are grouped into logical sections (Configuration → Connections →
// Databases → per-table checks) so failures are immediately obvious without
// reading a flat list of 20+ items.
//
// TTY mode: full ANSI colours with section headers and result banner.
// Non-TTY mode: plain indented text, suitable for piped output.
func PrintValidationResult(steps []ValidationStep, passed bool) {
	tty := IsTTY()
	lw := lineWidth

	failCount := 0
	for _, s := range steps {
		if !s.Passed {
			failCount++
		}
	}

	if tty {
		fmt.Printf("\n  %sValidation%s  %s\n\n", AnsiBold, AnsiReset, AnsiCyan+strings.Repeat("━", lw-14)+AnsiReset)
	} else {
		fmt.Println("\nValidation")
		fmt.Println(strings.Repeat("-", lw))
	}

	// Group steps into sections
	groups := groupSteps(steps)
	for _, g := range groups {
		if tty {
			fmt.Printf("    %s%s%s\n", AnsiBold, g.title, AnsiReset)
		} else {
			fmt.Printf("  %s\n", g.title)
		}
		for _, step := range g.steps {
			printStep(step, tty)
		}
		fmt.Println()
	}

	// Result banner
	if tty {
		fmt.Println("  " + AnsiCyan + strings.Repeat("━", lw) + AnsiReset)
		if passed {
			fmt.Printf("  Result:  %s%sPASSED%s\n\n", AnsiBold, AnsiGreen, AnsiReset)
		} else {
			plural := "s"
			if failCount == 1 {
				plural = ""
			}
			fmt.Printf("  Result:  %s%sFAILED%s  (%d error%s)\n\n",
				AnsiBold, AnsiRed, AnsiReset, failCount, plural)
		}
	} else {
		fmt.Println(strings.Repeat("-", lw))
		if passed {
			fmt.Println("Result: PASSED")
		} else {
			fmt.Printf("Result: FAILED (%d error(s))\n", failCount)
		}
		fmt.Println()
	}
}

func printStep(step ValidationStep, tty bool) {
	label := stepLabel(step.Name)
	if tty {
		if step.Passed {
			fmt.Printf("      %s✓%s  %s\n", AnsiGreen, AnsiReset, label)
		} else {
			fmt.Printf("      %s✗%s  %s   %s→ %s%s\n",
				AnsiRed, AnsiReset, label, AnsiRed, step.Message, AnsiReset)
		}
	} else {
		if step.Passed {
			fmt.Printf("    ✓ %s\n", label)
		} else {
			fmt.Printf("    ✗ %s — %s\n", label, step.Message)
		}
	}
}

// stepGroup is a named collection of validation steps.
type stepGroup struct {
	title string
	steps []ValidationStep
}

// groupSteps organises a flat validation step list into labelled sections.
func groupSteps(steps []ValidationStep) []stepGroup {
	var groups []stepGroup
	cur := ""

	addStep := func(title string, step ValidationStep) {
		if title != cur {
			groups = append(groups, stepGroup{title: title})
			cur = title
		}
		groups[len(groups)-1].steps = append(groups[len(groups)-1].steps, step)
	}

	for _, step := range steps {
		addStep(sectionTitle(step.Name), step)
	}
	return groups
}

// sectionTitle maps a step name to its display section heading.
func sectionTitle(name string) string {
	switch {
	case strings.HasPrefix(name, "Configuration"):
		return "Configuration"
	case strings.Contains(name, "Connection"):
		return "Connections"
	case strings.Contains(name, "Database"):
		return "Databases"
	case strings.HasPrefix(name, "Table Source") || strings.HasPrefix(name, "Table Destination"):
		return "  " + extractTable(name) + "  —  Existence"
	case strings.HasPrefix(name, "Source Describe") || strings.HasPrefix(name, "Destination Describe"):
		return "  " + extractTable(name) + "  —  Schema Fetch"
	case strings.HasPrefix(name, "Date Column"):
		return "  " + extractTable(name) + "  —  Date Column"
	case strings.HasPrefix(name, "Schema Comparison"):
		return "  " + extractTable(name) + "  —  Schema"
	default:
		return "General"
	}
}

// extractTable pulls the table name from step names like "Table Source Exists: db.tbl".
func extractTable(name string) string {
	if idx := strings.LastIndex(name, ": "); idx >= 0 {
		return name[idx+2:]
	}
	if idx := strings.LastIndex(name, " "); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

// stepLabel returns a concise display label, stripping the step-type prefix.
var stepLabelReplacements = []struct{ prefix, label string }{
	{"Configuration Validation", "Config file"},
	{"Source Connection Ping", "Source ping"},
	{"Destination Connection Ping", "Destination ping"},
	{"Source Database Exists: ", "Source DB          "},
	{"Destination Database Exists: ", "Destination DB     "},
	{"Table Source Exists: ", "Source table       "},
	{"Table Destination Exists: ", "Destination table  "},
	{"Source Describe Table: ", "Source schema      "},
	{"Destination Describe Table: ", "Destination schema "},
	{"Date Column Exists: ", "Exists             "},
	{"Date Column Supported: ", "Type supported     "},
	{"Date Column Match: ", "Type match         "},
	{"Schema Comparison: ", "Column comparison  "},
}

func stepLabel(name string) string {
	for _, r := range stepLabelReplacements {
		if name == r.prefix {
			return r.label
		}
		if strings.HasPrefix(name, r.prefix) {
			suffix := strings.TrimPrefix(name, r.prefix)
			return r.label + suffix
		}
	}
	return name
}
