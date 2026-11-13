// Command log-catalog audits explicit production events and writes their catalog.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type event struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Channel string `json:"channel"`
	Level   string `json:"level"`
	ID      string `json:"id"`
	Message string `json:"message"`
}

func literal(e ast.Expr) string {
	if x, ok := e.(*ast.BasicLit); ok && x.Kind == token.STRING {
		s, _ := strconv.Unquote(x.Value)
		return s
	}
	return ""
}
func main() {
	var events []event
	var problems []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && (d.Name() == "desktop" || d.Name() == "bin" || d.Name() == "data-lab-linux" || d.Name() == "tools" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fs := token.NewFileSet()
		f, e := parser.ParseFile(fs, path, nil, 0)
		if e != nil {
			return e
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkg.Name == "log" && (strings.HasPrefix(sel.Sel.Name, "Print") || strings.HasPrefix(sel.Sel.Name, "Fatal")) {
				problems = append(problems, fmt.Sprint(fs.Position(c.Pos()), ": untyped event"))
			}
			if pkg.Name != "diagnostics" || (sel.Sel.Name != "Emit" && sel.Sel.Name != "Fatal") {
				return true
			}
			idIndex := 2
			channel := "gateway"
			level := "Error"
			if sel.Sel.Name == "Fatal" {
				idIndex = 0
			} else {
				channel = literal(c.Args[0])
				if l, ok := c.Args[1].(*ast.SelectorExpr); ok {
					level = l.Sel.Name
				} else {
					level = "explicit-variable"
				}
			}
			id, msg := literal(c.Args[idIndex]), literal(c.Args[idIndex+1])
			if !strings.HasPrefix(id, "HGW-") || msg == "" {
				problems = append(problems, fmt.Sprint(fs.Position(c.Pos()), ": event requires literal identifier and template"))
			}
			events = append(events, event{path, fs.Position(c.Pos()).Line, channel, level, id, msg})
			return true
		})
		return nil
	})
	if err != nil {
		panic(err)
	}
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(problems, "\n"))
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(events, "", "  ")
	if err := os.WriteFile("../docs/eventos-log.json", append(b, '\n'), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("%d explicit events audited\n", len(events))
}
