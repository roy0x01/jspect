// jspect — JavaScript formatter for daily terminal use.
//
// Install:
//
//	go install github.com/roy0x01/jspect@latest
//
// Usage:
//
//	jspect <input.js|url> [-o output.js]
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ── ANSI colors ───────────────────────────────────────────────────────────────

const (
	reset    = "\033[0m"
	cKeyword = "\033[38;5;213m"
	cString  = "\033[38;5;114m"
	cNum     = "\033[38;5;209m"
	cComment = "\033[38;5;240m"
	cIdent   = "\033[38;5;117m"
	cPunct   = "\033[38;5;247m"
	cBrace   = "\033[38;5;220m"
	cOp      = "\033[38;5;204m"
	cBold    = "\033[1m"
	cGreen   = "\033[38;5;114m"
	cRed     = "\033[38;5;196m"
	cYellow  = "\033[38;5;220m"
	cGrey    = "\033[38;5;240m"
)

// ── Token types ───────────────────────────────────────────────────────────────

type tokKind int

const (
	tWhitespace tokKind = iota
	tNewline
	tLineComment
	tBlockComment
	tString
	tTemplate
	tRegex
	tNumber
	tIdent
	tKeyword
	tBrace  // { } ( ) [ ]
	tOp
	tPunct  // , ; .
	tSpread // ...
	tColon  // :
	tUnknown
)

type token struct {
	kind tokKind
	val  string
}

var keywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "export": true, "extends": true, "finally": true, "for": true,
	"function": true, "if": true, "import": true, "in": true, "instanceof": true,
	"let": true, "new": true, "null": true, "of": true, "return": true, "static": true,
	"super": true, "switch": true, "throw": true, "true": true, "false": true,
	"try": true, "typeof": true, "undefined": true, "var": true, "void": true,
	"while": true, "with": true, "yield": true, "async": true, "await": true,
	"from": true, "as": true, "this": true,
}

// ── Tokenizer ─────────────────────────────────────────────────────────────────

func tokenize(src string) []token {
	var tokens []token
	i, n := 0, len(src)

	prevMeaningfulVal := func() string {
		for k := len(tokens) - 1; k >= 0; k-- {
			k2 := tokens[k].kind
			if k2 != tWhitespace && k2 != tNewline {
				return tokens[k].val
			}
		}
		return ""
	}
	prevMeaningfulKind := func() tokKind {
		for k := len(tokens) - 1; k >= 0; k-- {
			k2 := tokens[k].kind
			if k2 != tWhitespace && k2 != tNewline {
				return tokens[k].kind
			}
		}
		return tUnknown
	}

	for i < n {
		// newline
		if src[i] == '\n' || src[i] == '\r' {
			if src[i] == '\r' && i+1 < n && src[i+1] == '\n' {
				i++
			}
			tokens = append(tokens, token{tNewline, "\n"})
			i++
			continue
		}
		// whitespace
		if src[i] == ' ' || src[i] == '\t' {
			j := i
			for j < n && (src[j] == ' ' || src[j] == '\t') {
				j++
			}
			tokens = append(tokens, token{tWhitespace, src[i:j]})
			i = j
			continue
		}
		// line comment
		if i+1 < n && src[i] == '/' && src[i+1] == '/' {
			j := i
			for j < n && src[j] != '\n' && src[j] != '\r' {
				j++
			}
			tokens = append(tokens, token{tLineComment, src[i:j]})
			i = j
			continue
		}
		// block comment
		if i+1 < n && src[i] == '/' && src[i+1] == '*' {
			j := i + 2
			for j+1 < n && !(src[j] == '*' && src[j+1] == '/') {
				j++
			}
			if j+1 < n {
				j += 2
			}
			tokens = append(tokens, token{tBlockComment, src[i:j]})
			i = j
			continue
		}
		// template literal (don't recurse into ${} — treat as one token)
		if src[i] == '`' {
			j := i + 1
			depth := 0
			for j < n {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '$' && j+1 < n && src[j+1] == '{' {
					depth++
					j += 2
					continue
				}
				if src[j] == '}' && depth > 0 {
					depth--
					j++
					continue
				}
				if src[j] == '`' && depth == 0 {
					j++
					break
				}
				j++
			}
			tokens = append(tokens, token{tTemplate, src[i:j]})
			i = j
			continue
		}
		// string
		if src[i] == '"' || src[i] == '\'' {
			q := src[i]
			j := i + 1
			for j < n && src[j] != q {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '\n' {
					break
				}
				j++
			}
			if j < n && src[j] == q {
				j++
			}
			tokens = append(tokens, token{tString, src[i:j]})
			i = j
			continue
		}
		// spread operator ...
		if i+2 < n && src[i] == '.' && src[i+1] == '.' && src[i+2] == '.' {
			tokens = append(tokens, token{tSpread, "..."})
			i += 3
			continue
		}
		// regex
		if src[i] == '/' {
			pmk := prevMeaningfulKind()
			pmv := prevMeaningfulVal()
			isRegex := pmk == tUnknown || pmk == tOp || pmk == tKeyword ||
				pmv == "(" || pmv == "[" || pmv == "," || pmv == ";" ||
				pmv == "{" || pmv == "!" || pmv == "return" || pmv == "="
			if isRegex {
				j := i + 1
				inClass := false
				for j < n {
					if src[j] == '[' {
						inClass = true
					} else if src[j] == ']' {
						inClass = false
					} else if src[j] == '\\' {
						j++
					} else if src[j] == '/' && !inClass {
						j++
						break
					} else if src[j] == '\n' {
						break
					}
					j++
				}
				for j < n && (src[j] == 'g' || src[j] == 'i' || src[j] == 'm' ||
					src[j] == 's' || src[j] == 'u' || src[j] == 'y') {
					j++
				}
				tokens = append(tokens, token{tRegex, src[i:j]})
				i = j
				continue
			}
		}
		// number
		if src[i] >= '0' && src[i] <= '9' ||
			(src[i] == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9') {
			j := i
			for j < n && (src[j] >= '0' && src[j] <= '9' ||
				src[j] == '.' || src[j] == 'e' || src[j] == 'E' ||
				src[j] == 'x' || src[j] == 'X' || src[j] == '_' ||
				(src[j] >= 'a' && src[j] <= 'f') ||
				(src[j] >= 'A' && src[j] <= 'F') || src[j] == 'n') {
				j++
			}
			tokens = append(tokens, token{tNumber, src[i:j]})
			i = j
			continue
		}
		// identifier / keyword
		if src[i] == '_' || src[i] == '$' || unicode.IsLetter(rune(src[i])) {
			j := i
			for j < n && (src[j] == '_' || src[j] == '$' ||
				unicode.IsLetter(rune(src[j])) || (src[j] >= '0' && src[j] <= '9')) {
				j++
			}
			word := src[i:j]
			if keywords[word] {
				tokens = append(tokens, token{tKeyword, word})
			} else {
				tokens = append(tokens, token{tIdent, word})
			}
			i = j
			continue
		}
		// braces
		if src[i] == '{' || src[i] == '}' || src[i] == '(' ||
			src[i] == ')' || src[i] == '[' || src[i] == ']' {
			tokens = append(tokens, token{tBrace, string(src[i])})
			i++
			continue
		}
		// punctuation (single dot handled after spread check above)
		if src[i] == ',' || src[i] == ';' || src[i] == '.' {
			tokens = append(tokens, token{tPunct, string(src[i])})
			i++
			continue
		}
		// operators — longest match first
		ops := []string{
			"===", "!==", "**=", "||=", "&&=", "??=", ">>>=",
			"?.", "**", "++", "--", "<<", ">>", ">>>", "<=", ">=",
			"==", "!=", "=>", "&&", "||", "??",
			"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=",
		}
		matched := false
		for _, op := range ops {
			if strings.HasPrefix(src[i:], op) {
				tokens = append(tokens, token{tOp, op})
				i += len(op)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if src[i] == ':' {
			tokens = append(tokens, token{tColon, ":"})
			i++
			continue
		}
		if strings.ContainsRune("=<>!+-*/%&|^~?/", rune(src[i])) {
			tokens = append(tokens, token{tOp, string(src[i])})
			i++
			continue
		}
		tokens = append(tokens, token{tUnknown, string(src[i])})
		i++
	}
	return tokens
}

// ── JSON pretty-printer ───────────────────────────────────────────────────────

func prettyJSON(raw string) (string, bool) {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw, false
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw, false
	}
	return string(out), true
}

func unquoteJS(s string) string {
	if len(s) < 2 {
		return s
	}
	inner := s[1 : len(s)-1]
	r := strings.NewReplacer(
		`\"`, `"`,
		`\'`, `'`,
		`\n`, "\n",
		`\t`, "\t",
		`\r`, "\r",
		`\\`, `\`,
	)
	return r.Replace(inner)
}

func expandJSONParse(tokens []token) []token {
	out := make([]token, 0, len(tokens))
	n := len(tokens)

	isMeaningful := func(k tokKind) bool {
		return k != tWhitespace && k != tNewline
	}
	nextMeaningful := func(from int) (token, int) {
		for i := from; i < n; i++ {
			if isMeaningful(tokens[i].kind) {
				return tokens[i], i
			}
		}
		return token{}, -1
	}

	for i := 0; i < n; i++ {
		tok := tokens[i]
		if tok.kind == tIdent && tok.val == "JSON" {
			dot, di := nextMeaningful(i + 1)
			if dot.val != "." {
				out = append(out, tok)
				continue
			}
			parse, pi := nextMeaningful(di + 1)
			if parse.val != "parse" {
				out = append(out, tok)
				continue
			}
			openParen, oi := nextMeaningful(pi + 1)
			if openParen.val != "(" {
				out = append(out, tok)
				continue
			}
			strTok, si := nextMeaningful(oi + 1)
			if strTok.kind != tString && strTok.kind != tTemplate {
				out = append(out, tok)
				continue
			}
			closeParen, ci := nextMeaningful(si + 1)
			if closeParen.val != ")" {
				out = append(out, tok)
				continue
			}
			inner := unquoteJS(strTok.val)
			pretty, ok := prettyJSON(inner)
			if !ok {
				out = append(out, tok)
				continue
			}
			var buf bytes.Buffer
			buf.WriteByte('`')
			buf.WriteString(pretty)
			buf.WriteByte('`')
			out = append(out, tok)
			out = append(out, tokens[di])
			out = append(out, parse)
			out = append(out, openParen)
			out = append(out, token{kind: tTemplate, val: buf.String()})
			out = append(out, closeParen)
			i = ci
			continue
		}
		out = append(out, tok)
	}
	return out
}

// ── Printer ───────────────────────────────────────────────────────────────────

func printTokens(tokens []token) string {
	const tab = "  "
	var sb strings.Builder
	indent := 0

	// Context stack: 'b'=block{} 'p'=paren() 'a'=array[]
	type frame struct{ kind byte }
	var stack []frame
	push := func(k byte) { stack = append(stack, frame{k}) }
	pop := func() byte {
		if len(stack) == 0 {
			return 0
		}
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return f.kind
	}
	topKind := func() byte {
		if len(stack) == 0 {
			return 0
		}
		return stack[len(stack)-1].kind
	}

	nl := func() {
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat(tab, indent))
	}
	sp := func() { sb.WriteByte(' ') }

	// nextMeaningful returns the next non-whitespace/newline token from position.
	nextM := func(from int) (token, int) {
		for j := from; j < len(tokens); j++ {
			if tokens[j].kind != tWhitespace && tokens[j].kind != tNewline {
				return tokens[j], j
			}
		}
		return token{}, -1
	}

	atLineStart := true
	prevKind := tUnknown
	prevVal := ""
	afterCaseKW := false // tracks if we just saw case/default keyword
	inCaseBody := false  // tracks if we're inside a case body

	// Keywords that always start a new statement.
	stmtKW := map[string]bool{
		"const": true, "let": true, "var": true, "function": true,
		"class": true, "if": true, "for": true, "while": true,
		"do": true, "switch": true, "try": true, "return": true,
		"throw": true, "break": true, "continue": true,
		"import": true, "export": true, "debugger": true,
		"case": true, "default": true,
	}
	// Keywords that continue the previous line (no leading newline).
	contKW := map[string]bool{
		"else": true, "catch": true, "finally": true,
		"from": true, "as": true,
	}

	write := func(s string) {
		sb.WriteString(s)
		atLineStart = false
	}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.kind == tWhitespace || tok.kind == tNewline {
			continue
		}

		switch tok.val {

		// ── { ────────────────────────────────────────────────────────────────
		case "{":
			// Destructuring after const/let/var: no newline after {
			// Regular block: newline + indent after {
			if !atLineStart && prevVal != "(" && prevVal != "[" &&
				prevVal != "=>" && prevVal != "," && prevKind != tColon {
				sp()
			}
			write("{")
			push('b')
			// Check if this is a destructuring assignment (= follows the matching })
			// Simple heuristic: if prev was = or : or ( treat as value object
			indent++
			nl()
			atLineStart = true
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── } ────────────────────────────────────────────────────────────────
		case "}":
			// If still in a case body (no break), dedent it first.
			if inCaseBody {
				indent--
				inCaseBody = false
			}
			indent--
			if indent < 0 {
				indent = 0
			}
			pop()
			if !atLineStart {
				nl()
			}
			write("}")
			next, _ := nextM(i + 1)
			if next.val == "else" || next.val == "catch" || next.val == "finally" {
				sp()
			} else if next.val == "from" {
				sp()
			} else if next.kind == tColon {
				// ternary : after } — stay on same line with space
				sp()
			} else if next.val == ")" || next.val == "," || next.val == ";" ||
				next.val == "." || next.val == "" {
				// stay on same line
			} else {
				nl()
				atLineStart = true
			}
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── ( ────────────────────────────────────────────────────────────────
		case "(":
			// space before ( after =>, =, return, and control-flow keywords
			if prevVal == "=>" || prevVal == "=" || prevVal == "return" {
				sp()
			} else if prevKind == tKeyword &&
				(prevVal == "if" || prevVal == "for" || prevVal == "while" ||
					prevVal == "switch" || prevVal == "catch") {
				sp()
			}
			write("(")
			push('p')
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── ) ────────────────────────────────────────────────────────────────
		case ")":
			pop()
			write(")")
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── [ ────────────────────────────────────────────────────────────────
		case "[":
			// space before [ only if it's a fresh array literal, not indexing
			if prevKind != tIdent && prevVal != ")" && prevVal != "]" && !atLineStart {
				sp()
			}
			write("[")
			push('a')
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── ] ────────────────────────────────────────────────────────────────
		case "]":
			pop()
			write("]")
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── ; ────────────────────────────────────────────────────────────────
		case ";":
			write(";")
			// peek: is ; inside a for(;;) ?
			if topKind() == 'p' {
				sp()
			} else {
				nl()
				atLineStart = true
			}
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── , ────────────────────────────────────────────────────────────────
		case ",":
			write(",")
			if topKind() == 'b' {
				nl()
				atLineStart = true
			} else {
				sp()
			}
			prevKind, prevVal = tok.kind, tok.val
			continue

		// ── . ────────────────────────────────────────────────────────────────
		case ".":
			write(".")
			prevKind, prevVal = tok.kind, tok.val
			continue
		}

		// ── : colon ──────────────────────────────────────────────────────────────
		if tok.kind == tColon {
			ctx := topKind()
			// Case label or object key — no space before, handle differently.
			if ctx == 'b' || afterCaseKW {
				isCaseLabel := afterCaseKW
				afterCaseKW = false
				write(":")
				if isCaseLabel {
					indent++
					nl()
					atLineStart = true
					inCaseBody = true
				} else {
					sp()
				}
			} else {
				// ternary else branch — space before (unless } already added it) and after
				afterCaseKW = false
				if !atLineStart && prevVal != "}" {
					sp()
				}
				write(":")
				sp()
			}
			prevKind, prevVal = tok.kind, tok.val
			continue
		}

		// ── spread ───────────────────────────────────────────────────────────
		if tok.kind == tSpread {
			write("...")
			prevKind, prevVal = tok.kind, tok.val
			continue
		}

		// ── comments ─────────────────────────────────────────────────────────
		if tok.kind == tLineComment {
			if !atLineStart {
				sp()
			}
			write(tok.val)
			nl()
			atLineStart = true
			prevKind, prevVal = tok.kind, tok.val
			continue
		}
		if tok.kind == tBlockComment {
			if !atLineStart {
				sp()
			}
			write(tok.val)
			sp()
			prevKind, prevVal = tok.kind, tok.val
			continue
		}

		// ── keywords ─────────────────────────────────────────────────────────
		if tok.kind == tKeyword {
			// After ".", keywords are method names — treat as regular identifiers.
			if prevVal == "." {
				write(tok.val)
				prevKind, prevVal = tIdent, tok.val
				continue
			}
			if tok.val == "case" || tok.val == "default" {
				afterCaseKW = true
			}
			switch {
			case contKW[tok.val]:
				// else/catch/finally/from/as — stay on same line, ensure space
				// } handler already added sp(), skip if after } or .
				if !atLineStart && prevVal != "}" && prevVal != "." {
					sp()
				}
			case tok.val == "async":
				// async: new line if starting statement
				if !atLineStart && prevVal != "=" && prevVal != "(" &&
					prevVal != "," && prevVal != "=>" && prevKind != tOp {
					nl()
					atLineStart = true
				}
			case stmtKW[tok.val]:
				// Dedent before break and before case/default in case body.
				if inCaseBody && (tok.val == "break" || tok.val == "case" || tok.val == "default" || tok.val == "return") {
					if tok.val == "break" || tok.val == "return" {
						// keep body indent for break/return
					} else {
						indent--
						inCaseBody = false
					}
				}
				if !atLineStart && prevVal != "=" && prevVal != "(" &&
					prevVal != "," && prevVal != "=>" && prevVal != "async" &&
					prevVal != "default" && prevVal != "export" && prevKind != tOp {
					nl()
					atLineStart = true
				}
				// After break, dedent out of case body.
				if tok.val == "break" && inCaseBody {
					// will dedent after break is written
				}
			}
		}

		// ── operators — spacing rules ─────────────────────────────────────────
		if tok.kind == tOp {
			switch tok.val {
			case "++", "--":
				// postfix: no space before; prefix: no space after (handled below)
				if prevKind == tIdent || prevVal == ")" || prevVal == "]" {
					write(tok.val) // postfix
					prevKind, prevVal = tok.kind, tok.val
					continue
				}
				// prefix — emit below with no leading space
			case "!", "~":
				// unary — no leading space
			case "?.":
				// optional chaining — no spaces
				write(tok.val)
				prevKind, prevVal = tok.kind, tok.val
				continue
			case "?":
				// ternary — space before
				if !atLineStart {
					sp()
				}
			default:
				if !atLineStart {
					sp()
				}
			}
		}

		// ── default emit ─────────────────────────────────────────────────────
		if !atLineStart {
			switch {
			case prevKind == tOp && prevVal != "++" && prevVal != "--" &&
				prevVal != "!" && prevVal != "~" && prevVal != "?.":
				sp()
			case prevKind == tKeyword:
				sp()
			case (prevKind == tIdent || prevKind == tNumber) &&
				(tok.kind == tIdent || tok.kind == tKeyword || tok.kind == tNumber):
				sp()
			case prevKind == tKeyword && tok.kind == tString:
				sp()
			case tok.kind == tString && prevKind == tKeyword:
				sp()
			}
		}

		write(tok.val)
		// After break in case body: dedent for next case label.
		if tok.kind == tKeyword && tok.val == "break" && inCaseBody {
			indent--
			inCaseBody = false
		}
		prevKind, prevVal = tok.kind, tok.val
	}

	// Tidy: trim trailing whitespace, collapse blanks, remove blank lines before }.
	out := strings.TrimSpace(sb.String()) + "\n"
	lines := strings.Split(out, "\n")
	var clean []string
	blanks := 0
	for idx, l := range lines {
		l = strings.TrimRight(l, " \t")
		// Skip blank lines immediately before a closing brace line.
		if l == "" {
			nextNonBlank := ""
			for _, nl := range lines[idx+1:] {
				if strings.TrimSpace(nl) != "" {
					nextNonBlank = strings.TrimSpace(nl)
					break
				}
			}
			if nextNonBlank == "}" || nextNonBlank == "})" || nextNonBlank == "}," || nextNonBlank == "};" {
				continue
			}
			blanks++
			if blanks <= 1 {
				clean = append(clean, l)
			}
		} else {
			blanks = 0
			clean = append(clean, l)
		}
	}
	return strings.Join(clean, "\n") + "\n"
}

// ── Colorizer ─────────────────────────────────────────────────────────────────

func colorize(src string) string {
	tokens := tokenize(src)
	var sb strings.Builder
	for i, tok := range tokens {
		switch tok.kind {
		case tKeyword:
			sb.WriteString(cKeyword + tok.val + reset)
		case tString, tTemplate, tRegex:
			sb.WriteString(cString + tok.val + reset)
		case tNumber:
			sb.WriteString(cNum + tok.val + reset)
		case tLineComment, tBlockComment:
			sb.WriteString(cComment + tok.val + reset)
		case tIdent:
			next := i + 1
			for next < len(tokens) && tokens[next].kind == tWhitespace {
				next++
			}
			if next < len(tokens) && tokens[next].val == "(" {
				sb.WriteString(cIdent + tok.val + reset)
			} else {
				sb.WriteString(tok.val)
			}
		case tBrace:
			sb.WriteString(cBrace + tok.val + reset)
		case tOp:
			sb.WriteString(cOp + tok.val + reset)
		case tPunct, tSpread:
			sb.WriteString(cPunct + tok.val + reset)
		default:
			sb.WriteString(tok.val)
		}
	}
	return sb.String()
}

// ── UI helpers ────────────────────────────────────────────────────────────────

func header(file string) {
	fmt.Fprintf(os.Stderr, "%s%s▶ %s%s  %s[JS]%s\n",
		cBold, cYellow, filepath.Base(file), reset, cGrey, reset)
}
func ok(msg string)   { fmt.Fprintf(os.Stderr, "  %s✔%s %s\n", cGreen, reset, msg) }
func fail(msg string) { fmt.Fprintf(os.Stderr, "  %s✘%s %s\n", cRed, reset, msg) }

// ── URL fetcher ──────────────────────────────────────────────────────────────

func fetchURL(rawURL string) ([]byte, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	ct := resp.Header.Get("Content-Type")
	// Accept JS content types and also plain text (common for CDN-served scripts).
	if ct != "" &&
		!strings.Contains(ct, "javascript") &&
		!strings.Contains(ct, "text/plain") &&
		!strings.Contains(ct, "application/octet-stream") {
		return nil, fmt.Errorf("unexpected content-type: %s", ct)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}
	return data, nil
}

// ── Usage ─────────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprint(os.Stderr, `
  jspect — JavaScript formatter for daily terminal use

  Install:
    go install github.com/roy0x01/jspect@latest

  Usage:
    jspect <input.js|url> [-o output.js]

  Default:
    Formats and prints colored output to terminal.
    Works on both normal and minified JS.
    Accepts local files or HTTP/HTTPS URLs.
    Input is never modified.

  Flags:
    -o <file>        write formatted plain output to file
    --analyze        scan for endpoints, URLs, secrets, IPs, and more
    --config <file>  path to custom analyze config (default: ~/.jspect/analyze.conf)
    --init-config    write default config to ~/.jspect/analyze.conf and exit

  Examples:
    jspect app.js
    jspect app.min.js -o app.js
    jspect https://cdn.example.com/lib.js
    jspect app.js --analyze
    jspect app.js --analyze --config ~/my-rules.yaml
    jspect --init-config

`)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	var input, output, configPath string
	var doAnalyze, initConfig bool

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o":
			if i+1 >= len(args) {
				fail("-o requires a filename")
				os.Exit(1)
			}
			i++
			output = args[i]
		case strings.HasPrefix(a, "-o="):
			output = a[3:]
		case a == "--analyze" || a == "-analyze":
			doAnalyze = true
		case a == "--init-config":
			initConfig = true
		case a == "--config" || a == "-config":
			if i+1 >= len(args) {
				fail("--config requires a file path")
				os.Exit(1)
			}
			i++
			configPath = args[i]
		case strings.HasPrefix(a, "--config="):
			configPath = a[9:]
		case a == "--help" || a == "-h":
			usage()
			os.Exit(0)
		case strings.HasPrefix(a, "-"):
			fail("unknown flag: " + a)
			usage()
			os.Exit(1)
		default:
			if input != "" {
				fail("only one input file at a time")
				os.Exit(1)
			}
			input = a
		}
	}

	// --config only matters alongside --analyze or --init-config — warn if
	// it would otherwise be silently ignored.
	if configPath != "" && !doAnalyze && !initConfig {
		fmt.Fprintf(os.Stderr, "  %s⚠%s  --config has no effect without --analyze or --init-config\n", cYellow, reset)
	}

	// --init-config: write default config and exit
	if initConfig {
		p := defaultConfigPath()
		if configPath != "" {
			p = configPath
		}
		if err := writeDefaultConfig(p); err != nil {
			fail("could not write config: " + err.Error())
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "  %s✔%s config written → %s\n", cGreen, reset, p)
		os.Exit(0)
	}

	if input == "" {
		fail("no input file specified")
		usage()
		os.Exit(1)
	}

	var src []byte
	var err error
	isURL := strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://")

	if isURL {
		fmt.Fprintf(os.Stderr, "  %s↓%s fetching %s\n", cGrey, reset, input)
		src, err = fetchURL(input)
		if err != nil {
			fail(err.Error())
			os.Exit(1)
		}
		// Derive a display name from the URL path.
		parts := strings.Split(strings.TrimRight(input, "/"), "/")
		name := parts[len(parts)-1]
		if name == "" || !strings.Contains(name, ".") {
			name = "remote.js"
		}
		header(name)
	} else {
		ext := strings.ToLower(filepath.Ext(input))
		if ext != ".js" && ext != ".mjs" && ext != ".cjs" {
			fail("unsupported file type: " + ext + " (expected .js, .mjs, .cjs)")
			os.Exit(1)
		}
		src, err = os.ReadFile(input)
		if err != nil {
			fail("cannot read file: " + err.Error())
			os.Exit(1)
		}
		header(input)
	}

	tokens := tokenize(string(src))
	tokens = expandJSONParse(tokens)
	formatted := printTokens(tokens)
	os.Stdout.WriteString(colorize(formatted))

	// --analyze: run pattern scan
	if doAnalyze {
		cp := configPath
		if cp == "" {
			cp = defaultConfigPath()
		}
		// Auto-create the default config on first use — no manual --init-config needed.
		if _, statErr := os.Stat(cp); os.IsNotExist(statErr) {
			if err := writeDefaultConfig(cp); err != nil {
				fail("could not create default config: " + err.Error())
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "  %s✔%s default config created → %s\n", cGreen, reset, cp)
		}
		cfg, err := parseAnalyzeConfig(cp)
		if err != nil {
			fail("config: " + err.Error())
		} else {
			findings := runAnalysis(formatted, cfg)
			printFindings(findings, input)
		}
	}

	if output != "" {
		if err := os.WriteFile(output, []byte(formatted), 0644); err != nil {
			fail("cannot write " + output + ": " + err.Error())
			os.Exit(1)
		}
		ok("saved → " + output)
	}
}
