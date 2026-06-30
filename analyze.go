package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Config format (.jspect/analyze.conf) ──────────────────────────────────────
//
// Simple key=value per rule, blank line separates rules:
//
//   name     = JWT Token
//   category = Credentials
//   severity = high
//   pattern  = eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}
//
//   # lines starting with # are comments

type Rule struct {
	Name     string
	Category string
	Pattern  string
	Severity string
	re       *regexp.Regexp
}

type AnalyzeConfig struct {
	Rules []Rule
}

// ── Parser ────────────────────────────────────────────────────────────────────

func parseAnalyzeConfig(path string) (*AnalyzeConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &AnalyzeConfig{}
	cur := &Rule{}
	sc := bufio.NewScanner(f)

	flush := func() {
		if cur.Pattern == "" {
			cur = &Rule{}
			return
		}
		re, err := regexp.Compile(cur.Pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s⚠%s  rule %q: bad regex: %v\n", cYellow, reset, cur.Name, err)
			cur = &Rule{}
			return
		}
		cur.re = re
		if cur.Severity == "" {
			cur.Severity = "info"
		}
		if cur.Category == "" {
			cur.Category = "General"
		}
		cfg.Rules = append(cfg.Rules, *cur)
		cur = &Rule{}
	}

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		// Blank line = end of rule block.
		if trimmed == "" {
			flush()
			continue
		}
		// Comment.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		eqIdx := strings.IndexByte(trimmed, '=')
		if eqIdx < 0 {
			continue
		}
		k := strings.TrimSpace(trimmed[:eqIdx])
		v := strings.TrimSpace(trimmed[eqIdx+1:])

		switch k {
		case "name":
			cur.Name = v
		case "category":
			cur.Category = v
		case "severity":
			cur.Severity = v
		case "pattern":
			cur.Pattern = v
		}
	}
	flush() // flush last rule

	return cfg, sc.Err()
}

// ── Default config path ───────────────────────────────────────────────────────

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".jspect/analyze.conf"
	}
	return filepath.Join(home, ".jspect", "analyze.conf")
}

// ── Finding ───────────────────────────────────────────────────────────────────

type finding struct {
	rule    Rule
	match   string
	context string
}

// ── Runner ────────────────────────────────────────────────────────────────────

func runAnalysis(src string, cfg *AnalyzeConfig) []finding {
	var findings []finding
	seen := map[string]bool{}

	lines := strings.Split(src, "\n")
	for lineNum, line := range lines {
		for _, rule := range cfg.Rules {
			for _, sm := range rule.re.FindAllStringSubmatch(line, -1) {
				// Prefer capture group 1 if the pattern defines one (cleaner display
				// value, e.g. just the path instead of the whole fetch(...) call).
				m := sm[0]
				display := sm[0]
				if len(sm) > 1 && sm[1] != "" {
					display = sm[1]
				}

				key := rule.Name + ":" + m
				if seen[key] {
					continue
				}
				seen[key] = true

				ctx := fmt.Sprintf("line %d: %s", lineNum+1, strings.TrimSpace(line))
				if len(ctx) > 140 {
					ctx = ctx[:140] + "…"
				}

				display = strings.Trim(display, `"'`+"`"+`.,;) `)
				findings = append(findings, finding{
					rule:    rule,
					match:   display,
					context: ctx,
				})
			}
		}
	}
	return findings
}

// ── Printer ───────────────────────────────────────────────────────────────────

var severityColor = map[string]string{
	"critical": "\033[38;5;196m",
	"high":     "\033[38;5;209m",
	"medium":   "\033[38;5;220m",
	"info":     "\033[38;5;117m",
}

var severityOrder = map[string]int{
	"critical": 0, "high": 1, "medium": 2, "info": 3,
}

func printFindings(findings []finding, target string) {
	if len(findings) == 0 {
		fmt.Fprintf(os.Stdout, "\n  %s✔  no findings%s\n\n", cGreen, reset)
		return
	}

	catMap := map[string][]finding{}
	for _, f := range findings {
		catMap[f.rule.Category] = append(catMap[f.rule.Category], f)
	}

	// Sort categories; put Credentials and Cloud Keys first.
	priorityCat := map[string]int{
		"Credentials": 0, "Cloud Keys": 1, "Endpoints": 2,
		"GraphQL": 3, "URLs": 4, "Network": 5, "Debug": 6,
	}
	var cats []string
	for c := range catMap {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool {
		pi, pj := priorityCat[cats[i]], priorityCat[cats[j]]
		if pi != pj {
			if pi == 0 {
				return true
			}
			if pj == 0 {
				return false
			}
		}
		return cats[i] < cats[j]
	})

	for cat := range catMap {
		sort.Slice(catMap[cat], func(i, j int) bool {
			si := severityOrder[catMap[cat][i].rule.Severity]
			sj := severityOrder[catMap[cat][j].rule.Severity]
			if si != sj {
				return si < sj
			}
			return catMap[cat][i].rule.Name < catMap[cat][j].rule.Name
		})
	}

	fmt.Fprintf(os.Stdout, "\n%s%s▶ %s%s\n", cBold, cYellow, target, reset)
	fmt.Fprintf(os.Stdout, "%s%s%s\n\n", cGrey, strings.Repeat("─", 64), reset)

	total := 0
	for _, cat := range cats {
		fs := catMap[cat]
		fmt.Fprintf(os.Stdout, "  %s%s[%s]%s\n", cBold, cGrey, strings.ToUpper(cat), reset)
		for _, f := range fs {
			col := severityColor[f.rule.Severity]
			if col == "" {
				col = cGrey
			}
			sev := strings.ToUpper(f.rule.Severity)
			fmt.Fprintf(os.Stdout, "  %s[%-8s]%s  %-22s  %s\n",
				col, sev, reset, f.rule.Name, f.match)
			if f.context != "" {
				fmt.Fprintf(os.Stdout, "  %s             ↳ %s%s\n", cGrey, f.context, reset)
			}
		}
		fmt.Fprintln(os.Stdout)
		total += len(fs)
	}

	fmt.Fprintf(os.Stdout, "%s%s%s\n", cGrey, strings.Repeat("─", 64), reset)
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.rule.Severity]++
	}
	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "info"} {
		if n := counts[sev]; n > 0 {
			col := severityColor[sev]
			parts = append(parts, fmt.Sprintf("%s%d %s%s", col, n, sev, reset))
		}
	}
	fmt.Fprintf(os.Stdout, "  %s%d finding(s)%s  %s\n\n",
		cBold, total, reset, strings.Join(parts, "  "))
}

// ── Default config writer ─────────────────────────────────────────────────────
//
// The default rule set lives in analyze.conf at the repo root — a plain text
// file, easy to read, diff, and PR against. It's embedded into the binary at
// compile time via go:embed, so `go install` still works with zero external
// files needed at runtime. Editing the rules means editing analyze.conf and
// rebuilding — no Go code changes required.

//go:embed analyze.conf
var defaultConfigData []byte

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, defaultConfigData, 0644)
}
