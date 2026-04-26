package team

import (
	"fmt"
	"strings"
)

// FormatSpeciesEval formats a SpeciesEval as plain text for LLM consumption.
func FormatSpeciesEval(ev SpeciesEval) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", ev.Name, strings.Join(ev.Types, "/"))
	fmt.Fprintf(&b, "Role: %s | Speed: %s\n", ev.StatRole, ev.SpeedTier)
	fmt.Fprintf(&b, "Bulk: physical %.0f | special %.0f\n", ev.PhysicalBulk, ev.SpecialBulk)
	tm := ev.TypeMatchup
	if len(tm.Immune) > 0 {
		fmt.Fprintf(&b, "Immune (0×):     %s\n", strings.Join(tm.Immune, ", "))
	}
	if len(tm.Quarter) > 0 {
		fmt.Fprintf(&b, "Quarter (0.25×): %s\n", strings.Join(tm.Quarter, ", "))
	}
	if len(tm.Half) > 0 {
		fmt.Fprintf(&b, "Resists (0.5×):  %s\n", strings.Join(tm.Half, ", "))
	}
	if len(tm.Double) > 0 {
		fmt.Fprintf(&b, "Weak (2×):       %s\n", strings.Join(tm.Double, ", "))
	}
	if len(tm.Quadruple) > 0 {
		fmt.Fprintf(&b, "Very weak (4×):  %s\n", strings.Join(tm.Quadruple, ", "))
	}
	if len(ev.OffensiveCoverage) > 0 {
		fmt.Fprintf(&b, "Offensive coverage (SE): %s\n", strings.Join(ev.OffensiveCoverage, ", "))
	}
	return b.String()
}

// FormatMemberEval formats a MemberEval as plain text for LLM consumption.
func FormatMemberEval(ev MemberEval) string {
	var b strings.Builder
	b.WriteString(FormatSpeciesEval(ev.SpeciesEval))
	b.WriteString("--- member analysis ---\n")
	var flags []string
	if ev.HasPriorityMove {
		flags = append(flags, "priority move")
	}
	if ev.HasSetupMove {
		flags = append(flags, "setup move")
	}
	if ev.HasRecoveryMove {
		flags = append(flags, "recovery move")
	}
	if ev.HasRedirection {
		flags = append(flags, "redirection")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "Flags: %s\n", strings.Join(flags, ", "))
	}
	if ev.MoveStatMismatch {
		fmt.Fprintf(&b, "EV warning: %s\n", ev.EVEfficiency)
	} else {
		fmt.Fprintf(&b, "EV efficiency: %s\n", ev.EVEfficiency)
	}
	return b.String()
}
