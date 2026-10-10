package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestTelemetryHelpCoversDispatcher(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "telemetry.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]bool{}
	add := func(expr ast.Expr) {
		literal, ok := expr.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return
		}
		name, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		commands[name] = true
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "runTelemetry" {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BinaryExpr:
				if name, ok := node.X.(*ast.Ident); ok && name.Name == "subcommand" && node.Op == token.EQL {
					add(node.Y)
				}
			case *ast.SwitchStmt:
				if name, ok := node.Tag.(*ast.Ident); ok && name.Name == "subcommand" {
					for _, statement := range node.Body.List {
						for _, expr := range statement.(*ast.CaseClause).List {
							add(expr)
						}
					}
				}
			}
			return true
		})
	}
	if len(commands) == 0 {
		t.Fatal("no telemetry dispatcher commands discovered")
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"telemetry", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help code=%d stderr=%s", code, &stderr)
	}
	usage, _, _ := strings.Cut(stdout.String(), "\n")
	_, subcommands, found := strings.Cut(stdout.String(), "Subcommands:\n")
	if !found {
		t.Fatal("help missing Subcommands block")
	}
	for command := range commands {
		if !strings.Contains(usage, command) {
			t.Errorf("usage missing dispatcher command %q: %s", command, usage)
		}
		listed := false
		for _, line := range strings.Split(subcommands, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == command {
				listed = true
			}
		}
		if !listed {
			t.Errorf("Subcommands block missing dispatcher command %q", command)
		}
	}
}
