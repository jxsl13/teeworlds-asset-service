// Package twcfg parses Teeworlds / DDNet configuration files.
//
// The grammar matches the original C++ console parser from the DDNet and
// Teeworlds engines (src/engine/shared/console.cpp):
//
//	line       = statement { ";" statement }
//	statement  = command { argument }
//	argument   = quoted | unquoted
//	quoted     = '"' { char | '\\' '"' | '\\' '\\' } '"'
//	unquoted   = <non-whitespace run>
//	comment    = "#" <rest of line>  (outside quotes)
//
// The [exec] command is resolved by recursively parsing the referenced file
// through a caller-supplied resolver, and the resulting commands are inlined
// (flattened) into the output so no nested structure is produced.
package twcfg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Command represents a single parsed command with its arguments.
type Command struct {
	// Name is the command / setting name (e.g. "sv_name", "bind", "add_vote").
	Name string
	// Args contains every argument that follows the command name.
	Args []string
}

// String returns the command formatted as a single config line.  String values
// that contain whitespace, quotes or backslashes are quoted and escaped.
func (c Command) String() string {
	var b strings.Builder
	b.WriteString(c.Name)
	for _, a := range c.Args {
		b.WriteByte(' ')
		if needsQuoting(a) {
			b.WriteByte('"')
			b.WriteString(escapeArg(a))
			b.WriteByte('"')
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}

// Resolver opens a referenced config file by name and returns its contents as
// an [io.Reader].  It is called when an "exec" command is encountered.
// Return [ErrSkipExec] to silently skip a particular exec without error.
// Returning any other error aborts parsing.
type Resolver func(filename string) (io.Reader, error)

// ErrSkipExec can be returned by a [Resolver] to silently ignore an exec
// directive without aborting the parse.
var ErrSkipExec = errors.New("skip exec")

// Parse reads a Teeworlds / DDNet config from r and returns the flat list of
// commands.  exec directives are resolved and flattened via resolve.
// If resolve is nil, exec commands are returned as regular commands without
// expansion.
func Parse(r io.Reader, resolve Resolver) ([]Command, error) {
	return parseReader(r, resolve, 0)
}

// parseReader is the internal recursive entry point.  depth tracks exec
// nesting so we can enforce the same FILE_RECURSION_LIMIT (16) as DDNet.
func parseReader(r io.Reader, resolve Resolver, depth int) ([]Command, error) {
	const maxExecDepth = 16 // IConsole::FILE_RECURSION_LIMIT
	if depth > maxExecDepth {
		return nil, fmt.Errorf("twcfg: exec recursion limit (%d) exceeded", maxExecDepth)
	}

	var cmds []Command
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		stmts, err := splitStatements(line)
		if err != nil {
			return cmds, fmt.Errorf("twcfg: line %d: %w", lineNo, err)
		}
		for _, stmt := range stmts {
			cmd, err := parseStatement(stmt)
			if err != nil {
				return cmds, fmt.Errorf("twcfg: line %d: %w", lineNo, err)
			}
			if cmd == nil {
				continue // empty statement (e.g. trailing semicolon)
			}

			// Flatten exec directives.
			if strings.EqualFold(cmd.Name, "exec") && resolve != nil && len(cmd.Args) > 0 {
				sub, err := resolveExec(cmd.Args[0], resolve, depth+1)
				if err != nil {
					return cmds, fmt.Errorf("twcfg: line %d: exec %q: %w", lineNo, cmd.Args[0], err)
				}
				cmds = append(cmds, sub...)
				continue
			}

			cmds = append(cmds, *cmd)
		}
	}
	if err := scanner.Err(); err != nil {
		return cmds, fmt.Errorf("twcfg: reading input: %w", err)
	}
	return cmds, nil
}

// resolveExec opens a file through the resolver and parses it recursively.
func resolveExec(filename string, resolve Resolver, depth int) ([]Command, error) {
	r, err := resolve(filename)
	if errors.Is(err, ErrSkipExec) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if rc, ok := r.(io.ReadCloser); ok {
		defer rc.Close()
	}
	return parseReader(r, resolve, depth)
}

// splitStatements splits a single line into individual statement strings
// respecting quoted regions and comments.  Semicolons inside quoted strings
// are not treated as separators.  A '#' outside a quoted string starts a
// comment that discards the rest of the line.
func splitStatements(line string) ([]string, error) {
	var stmts []string
	var cur strings.Builder
	inString := false
	i := 0
	for i < len(line) {
		ch := line[i]
		switch {
		case ch == '"':
			inString = !inString
			cur.WriteByte(ch)
			i++
		case ch == '\\' && inString:
			// Escape sequence inside a quoted string.
			cur.WriteByte(ch)
			i++
			if i < len(line) {
				cur.WriteByte(line[i])
				i++
			}
		case !inString && ch == ';':
			stmts = append(stmts, cur.String())
			cur.Reset()
			i++
		case !inString && ch == '#':
			// Comment — discard rest of line.
			i = len(line)
		default:
			cur.WriteByte(ch)
			i++
		}
	}
	if inString {
		return nil, errors.New("unterminated quoted string")
	}
	stmts = append(stmts, cur.String())
	return stmts, nil
}

// parseStatement parses a single statement (one command with arguments) from
// the raw string that has already been split by semicolons.  Returns nil if
// the statement is empty / whitespace-only.
func parseStatement(raw string) (*Command, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// Handle "mc;" prefix (multi-command prefix used by DDNet — the semicolons
	// have already been resolved at this point, so we just strip the prefix).
	raw = strings.TrimPrefix(raw, "mc;")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var cmd Command
	pos := 0

	for pos < len(raw) {
		// Skip whitespace between tokens.
		for pos < len(raw) && isSpace(raw[pos]) {
			pos++
		}
		if pos >= len(raw) {
			break
		}

		var arg string
		var err error
		if raw[pos] == '"' {
			arg, pos, err = parseQuoted(raw, pos)
			if err != nil {
				return nil, err
			}
		} else {
			arg, pos = parseUnquoted(raw, pos)
		}

		if cmd.Name == "" {
			cmd.Name = arg
		} else {
			cmd.Args = append(cmd.Args, arg)
		}
	}

	if cmd.Name == "" {
		return nil, nil
	}
	return &cmd, nil
}

// parseQuoted parses a double-quoted argument starting at raw[pos] == '"'.
// It handles the DDNet/Teeworlds escape conventions: \" and \\.
func parseQuoted(raw string, pos int) (string, int, error) {
	pos++ // skip opening quote
	var b strings.Builder
	for pos < len(raw) {
		ch := raw[pos]
		if ch == '\\' && pos+1 < len(raw) {
			next := raw[pos+1]
			if next == '"' || next == '\\' {
				b.WriteByte(next)
				pos += 2
				continue
			}
			// DDNet also writes the non-escape backslash literally.
			b.WriteByte(ch)
			pos++
			continue
		}
		if ch == '"' {
			pos++ // skip closing quote
			return b.String(), pos, nil
		}
		b.WriteByte(ch)
		pos++
	}
	return "", pos, errors.New("unterminated quoted string")
}

// parseUnquoted parses a plain (unquoted) argument starting at pos.
func parseUnquoted(raw string, pos int) (string, int) {
	start := pos
	for pos < len(raw) && !isSpace(raw[pos]) {
		pos++
	}
	return raw[start:pos], pos
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r'
}

// needsQuoting reports whether arg must be quoted when serialised.
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '"', '\\', ';', '#':
			return true
		}
	}
	return false
}

// escapeArg escapes backslashes and double quotes inside a value that will be
// wrapped in double quotes.
func escapeArg(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
