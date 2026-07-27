package main

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultExcerptBudget is the char budget used when --citation-excerpt is
// passed bare (no =N). Sized to fit a couple of lines of prose without
// overwhelming the trailing citation block.
const defaultExcerptBudget = 160

// excerptBudgetFlag backs --citation-excerpt[=N]. As a bool-style flag it can
// be passed bare (Set("true") → default budget) or with an explicit char
// count (Set("80")). A zero or negative count disables excerpts. The zero
// value (flag absent) means excerpts are off.
type excerptBudgetFlag struct {
	set    bool
	budget int
}

// IsBoolFlag lets the flag be passed bare (--citation-excerpt) as well as with
// a value (--citation-excerpt=80). It is what the arg splitter keys off to
// decide whether the following token is this flag's value.
func (f *excerptBudgetFlag) IsBoolFlag() bool { return true }

func (f *excerptBudgetFlag) String() string {
	if f == nil || !f.set {
		return ""
	}
	return strconv.Itoa(f.budget)
}

func (f *excerptBudgetFlag) Set(v string) error {
	f.set = true
	switch v {
	case "", "true":
		f.budget = defaultExcerptBudget
		return nil
	case "false":
		f.budget = 0
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("citation-excerpt: want a char count, got %q", v)
	}
	f.budget = n
	return nil
}

// Budget returns the effective excerpt char budget: 0 when the flag was not
// set or was set to a non-positive count (excerpts disabled).
func (f excerptBudgetFlag) Budget() int {
	if !f.set || f.budget <= 0 {
		return 0
	}
	return f.budget
}

// offToggleFlag backs a default-on column toggle like --citation-confidence and
// --citation-spans. The column shows by default; passing the flag bare, "=on",
// or "=true" keeps it on, while "=off"/"=false"/"=no" hides it. It is a
// bool-style flag so it can be written bare (which, being a no-op, is harmless)
// or with a value. Hidden reports whether the column should be suppressed.
type offToggleFlag struct {
	set    bool
	hidden bool
}

func (f *offToggleFlag) IsBoolFlag() bool { return true }

func (f *offToggleFlag) String() string {
	if f == nil || !f.set {
		return ""
	}
	if f.hidden {
		return "off"
	}
	return "on"
}

func (f *offToggleFlag) Set(v string) error {
	f.set = true
	switch strings.ToLower(v) {
	case "", "on", "true", "yes", "1":
		f.hidden = false
	case "off", "false", "no", "0":
		f.hidden = true
	default:
		return fmt.Errorf("want on/off, got %q", v)
	}
	return nil
}

// Hidden reports whether the toggle was set to suppress its column.
func (f offToggleFlag) Hidden() bool { return f.set && f.hidden }

// excerptBudgetSink is a --citation-excerpts adapter that writes the parsed
// budget directly into a target *int (the per-command chatRenderOptions field)
// rather than into an excerptBudgetFlag. It shares the same bare/=N parsing.
type excerptBudgetSink struct{ dst *int }

func (s *excerptBudgetSink) IsBoolFlag() bool { return true }

func (s *excerptBudgetSink) String() string {
	if s == nil || s.dst == nil || *s.dst <= 0 {
		return ""
	}
	return strconv.Itoa(*s.dst)
}

func (s *excerptBudgetSink) Set(v string) error {
	var f excerptBudgetFlag
	if err := f.Set(v); err != nil {
		return err
	}
	*s.dst = f.Budget()
	return nil
}

// offToggleSink is an offToggleFlag adapter that writes "hidden" into a target
// *bool (a per-command chatRenderOptions field). It shares the on/off parsing.
type offToggleSink struct{ hidden *bool }

func (s *offToggleSink) IsBoolFlag() bool { return true }

func (s *offToggleSink) String() string {
	if s == nil || s.hidden == nil || !*s.hidden {
		return ""
	}
	return "off"
}

func (s *offToggleSink) Set(v string) error {
	var f offToggleFlag
	if err := f.Set(v); err != nil {
		return err
	}
	*s.hidden = f.Hidden()
	return nil
}
