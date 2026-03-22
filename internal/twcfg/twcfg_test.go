package twcfg

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseSimpleCommand(t *testing.T) {
	input := "sv_name My Server"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "My", "Server")
}

func TestParseQuotedArgument(t *testing.T) {
	input := `sv_name "My Cool Server"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "My Cool Server")
}

func TestParseEscapedQuotes(t *testing.T) {
	input := `sv_motd "Say \"hello\" to everyone"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_motd", `Say "hello" to everyone`)
}

func TestParseEscapedBackslash(t *testing.T) {
	input := `sv_motd "path\\to\\file"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_motd", `path\to\file`)
}

func TestParseSemicolonSeparated(t *testing.T) {
	input := `sv_name "Test"; sv_port 8303; sv_max_clients 16`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
	assertCmd(t, cmds[1], "sv_port", "8303")
	assertCmd(t, cmds[2], "sv_max_clients", "16")
}

func TestParseSemicolonInsideQuotes(t *testing.T) {
	input := `sv_motd "rules; no cheating"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_motd", "rules; no cheating")
}

func TestParseComment(t *testing.T) {
	input := `sv_name "Test" # this is a comment`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
}

func TestParseCommentOnlyLine(t *testing.T) {
	input := "# full line comment\nsv_name Test"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
}

func TestParseHashInsideQuotes(t *testing.T) {
	input := `sv_motd "color #FF0000"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_motd", "color #FF0000")
}

func TestParseEmptyLines(t *testing.T) {
	input := "\n\n  \n\nsv_name Test\n\n"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
}

func TestParseMultipleLines(t *testing.T) {
	input := "sv_name \"My Server\"\nsv_port 8303\nsv_max_clients 16\npassword \"\"\nsv_map dm1"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 5 {
		t.Fatalf("expected 5 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "My Server")
	assertCmd(t, cmds[1], "sv_port", "8303")
	assertCmd(t, cmds[2], "sv_max_clients", "16")
	assertCmd(t, cmds[3], "password", "")
	assertCmd(t, cmds[4], "sv_map", "dm1")
}

func TestParseNoArgCommand(t *testing.T) {
	input := "shutdown"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "shutdown")
}

func TestParseBind(t *testing.T) {
	input := `bind f1 "+toggle cl_show_hook_coll_own 2 1"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "bind", "f1", "+toggle cl_show_hook_coll_own 2 1")
}

func TestParseNestedEscapes(t *testing.T) {
	// bind x "bind y \"say hello\""
	input := `bind x "bind y \"say hello\""`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "bind", "x", `bind y "say hello"`)
}

func TestParseAddVote(t *testing.T) {
	input := "add_vote \"Map: dm1\" \"sv_map dm1\"\nadd_vote \"Map: ctf5\" \"sv_map ctf5\"\nadd_vote \"Kick player\" \"kick %d\""
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "add_vote", "Map: dm1", "sv_map dm1")
	assertCmd(t, cmds[1], "add_vote", "Map: ctf5", "sv_map ctf5")
	assertCmd(t, cmds[2], "add_vote", "Kick player", "kick %d")
}

func TestParseToggle(t *testing.T) {
	input := `toggle sv_vote_kick 0 1`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "toggle", "sv_vote_kick", "0", "1")
}

func TestParseStrokeCommand(t *testing.T) {
	input := `bind mouse1 "+fire; +toggle cl_dummy_hammer 1 0"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "bind", "mouse1", "+fire; +toggle cl_dummy_hammer 1 0")
}

func TestParseExecWithResolver(t *testing.T) {
	files := map[string]string{
		"votes.cfg": "add_vote \"Map: dm1\" \"sv_map dm1\"\nadd_vote \"Map: ctf5\" \"sv_map ctf5\"",
	}
	resolver := mapResolver(files)
	input := "sv_name \"Test\"\nexec votes.cfg\nsv_port 8303"
	cmds, err := Parse(strings.NewReader(input), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
	assertCmd(t, cmds[1], "add_vote", "Map: dm1", "sv_map dm1")
	assertCmd(t, cmds[2], "add_vote", "Map: ctf5", "sv_map ctf5")
	assertCmd(t, cmds[3], "sv_port", "8303")
}

func TestParseNestedExec(t *testing.T) {
	files := map[string]string{
		"main.cfg":     "exec votes.cfg\nexec settings.cfg",
		"votes.cfg":    "add_vote \"Map: dm1\" \"sv_map dm1\"",
		"settings.cfg": "sv_port 8303",
	}
	resolver := mapResolver(files)
	input := "exec main.cfg"
	cmds, err := Parse(strings.NewReader(input), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "add_vote", "Map: dm1", "sv_map dm1")
	assertCmd(t, cmds[1], "sv_port", "8303")
}

func TestParseExecRecursionLimit(t *testing.T) {
	files := map[string]string{
		"loop.cfg": "exec loop.cfg",
	}
	resolver := mapResolver(files)
	_, err := Parse(strings.NewReader("exec loop.cfg"), resolver)
	if err == nil {
		t.Fatal("expected recursion limit error")
	}
	if !strings.Contains(err.Error(), "recursion limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseExecSkip(t *testing.T) {
	resolver := func(string) (io.Reader, error) {
		return nil, ErrSkipExec
	}
	input := "sv_name \"Test\"\nexec missing.cfg\nsv_port 8303"
	cmds, err := Parse(strings.NewReader(input), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
	assertCmd(t, cmds[1], "sv_port", "8303")
}

func TestParseExecNoResolver(t *testing.T) {
	input := "exec votes.cfg"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "exec", "votes.cfg")
}

func TestParseUnterminatedQuote(t *testing.T) {
	input := `sv_name "oops`
	_, err := Parse(strings.NewReader(input), nil)
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
}

func TestParseTrailingSemicolon(t *testing.T) {
	input := `sv_name "Test";`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
}

func TestParseEmptyQuotedString(t *testing.T) {
	input := `password ""`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "password", "")
}

func TestParseMixedQuotedUnquoted(t *testing.T) {
	input := `add_vote "Restart" restart`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "add_vote", "Restart", "restart")
}

func TestParseTabs(t *testing.T) {
	input := "sv_name\t\"Test\"\t8303"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test", "8303")
}

func TestParseIntSetting(t *testing.T) {
	input := "sv_max_clients 16"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCmd(t, cmds[0], "sv_max_clients", "16")
}

func TestParseColorSetting(t *testing.T) {
	input := "player_color_body $00FF00"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCmd(t, cmds[0], "player_color_body", "$00FF00")
}

func TestParseServerConfig(t *testing.T) {
	input := "# Server Configuration\nsv_name \"My DDNet Server\"\nsv_port 8303\nsv_max_clients 64\nsv_map \"dm1\"\npassword \"\"\n\n# Votes\nadd_vote \"Map: dm1\" \"sv_map dm1\"\nadd_vote \"Map: ctf5\" \"sv_map ctf5\"\nadd_vote \"Map: dm7\" \"sv_map dm7\"\n\n# Game settings\nsv_gametype dm; sv_timelimit 10\nsv_scorelimit 20\nsv_tournament_mode 0"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 12 {
		t.Fatalf("expected 12 commands, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "My DDNet Server")
	assertCmd(t, cmds[1], "sv_port", "8303")
	assertCmd(t, cmds[2], "sv_max_clients", "64")
	assertCmd(t, cmds[3], "sv_map", "dm1")
	assertCmd(t, cmds[4], "password", "")
	assertCmd(t, cmds[5], "add_vote", "Map: dm1", "sv_map dm1")
	assertCmd(t, cmds[6], "add_vote", "Map: ctf5", "sv_map ctf5")
	assertCmd(t, cmds[7], "add_vote", "Map: dm7", "sv_map dm7")
	assertCmd(t, cmds[8], "sv_gametype", "dm")
	assertCmd(t, cmds[9], "sv_timelimit", "10")
	assertCmd(t, cmds[10], "sv_scorelimit", "20")
	assertCmd(t, cmds[11], "sv_tournament_mode", "0")
}

func TestCommandString(t *testing.T) {
	tests := []struct {
		cmd  Command
		want string
	}{
		{
			cmd:  Command{Name: "sv_name", Args: []string{"Test"}},
			want: "sv_name Test",
		},
		{
			cmd:  Command{Name: "sv_name", Args: []string{"My Server"}},
			want: `sv_name "My Server"`,
		},
		{
			cmd:  Command{Name: "sv_motd", Args: []string{`Say "hi"`}},
			want: `sv_motd "Say \"hi\""`,
		},
		{
			cmd:  Command{Name: "password", Args: []string{""}},
			want: `password ""`,
		},
		{
			cmd:  Command{Name: "shutdown"},
			want: "shutdown",
		},
		{
			cmd:  Command{Name: "sv_motd", Args: []string{`path\to\file`}},
			want: `sv_motd "path\\to\\file"`,
		},
	}
	for _, tt := range tests {
		got := tt.cmd.String()
		if got != tt.want {
			t.Errorf("Command.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestParseExecCaseInsensitive(t *testing.T) {
	files := map[string]string{
		"test.cfg": "sv_port 8303",
	}
	resolver := mapResolver(files)
	input := "EXEC test.cfg"
	cmds, err := Parse(strings.NewReader(input), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_port", "8303")
}

func TestParseExecQuotedFilename(t *testing.T) {
	files := map[string]string{
		"my votes.cfg": "add_vote \"Map: dm1\" \"sv_map dm1\"",
	}
	resolver := mapResolver(files)
	input := `exec "my votes.cfg"`
	cmds, err := Parse(strings.NewReader(input), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "add_vote", "Map: dm1", "sv_map dm1")
}

func TestParseBackslashOutsideQuotes(t *testing.T) {
	input := `sv_map path\to\map`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_map", `path\to\map`)
}

func TestParseNonEscapeBackslashInQuotes(t *testing.T) {
	// Backslash followed by something other than \ or " is kept literally.
	input := `sv_motd "hello\nworld"`
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCmd(t, cmds[0], "sv_motd", `hello\nworld`)
}

func TestParseExecResolverError(t *testing.T) {
	resolver := func(string) (io.Reader, error) {
		return nil, errors.New("file not found")
	}
	_, err := Parse(strings.NewReader("exec missing.cfg"), resolver)
	if err == nil {
		t.Fatal("expected error from resolver")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseMultipleSemicolonsEmpty(t *testing.T) {
	input := ";;;sv_name Test;;;"
	cmds, err := Parse(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	assertCmd(t, cmds[0], "sv_name", "Test")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mapResolver(files map[string]string) Resolver {
	return func(filename string) (io.Reader, error) {
		content, ok := files[filename]
		if !ok {
			return nil, ErrSkipExec
		}
		return strings.NewReader(content), nil
	}
}

func assertCmd(t *testing.T, cmd Command, name string, args ...string) {
	t.Helper()
	if cmd.Name != name {
		t.Errorf("cmd.Name = %q, want %q", cmd.Name, name)
	}
	if len(args) == 0 {
		args = nil
	}
	if len(cmd.Args) != len(args) {
		t.Fatalf("cmd.Args count = %d, want %d; args = %v", len(cmd.Args), len(args), cmd.Args)
	}
	for i, a := range args {
		if cmd.Args[i] != a {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], a)
		}
	}
}
