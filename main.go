// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/gogrep"

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"os"
	"strings"

	"golang.org/x/tools/go/loader"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "need at least two args")
		os.Exit(2)
	}
	if err := grepArgs(args[0], args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grepArgs(expr string, args []string) error {
	exprNode, err := compileExpr(expr)
	if err != nil {
		return err
	}
	conf := loader.Config{
		TypeCheckFuncBodies: func(path string) bool {
			return false
		},
	}
	if _, err := conf.FromArgs(args, true); err != nil {
		return err
	}
	prog, err := conf.Load()
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	for _, pkg := range prog.InitialPackages() {
		for _, file := range pkg.Files {
			matches := search(exprNode, file)
			for _, n := range matches {
				fpos := conf.Fset.Position(n.Pos())
				if strings.HasPrefix(fpos.Filename, wd) {
					fpos.Filename = fpos.Filename[len(wd)+1:]
				}
				fmt.Printf("%v: %s\n", fpos, singleLinePrint(n))
			}
		}
	}
	return nil
}

func singleLinePrint(node ast.Node) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, token.NewFileSet(), node)
	return buf.String()
}

func compileExpr(expr string) (ast.Node, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse expr: %v", err)
	}
	var buf bytes.Buffer
	for _, t := range toks {
		var s string
		switch {
		case t.tok == tokWildcard:
			s = wildName(t.lit)
		case t.lit != "":
			s = t.lit
		default:
			buf.WriteString(t.tok.String())
		}
		buf.WriteString(s)
		buf.WriteByte(' ') // for e.g. consecutive idents
	}
	// trailing newlines can cause issues with commas
	exprStr := strings.TrimSpace(buf.String())
	node, err := parse(exprStr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse expr: %v", err)
	}
	return node, nil
}

func search(exprNode, node ast.Node) []ast.Node {
	var matches []ast.Node
	match := func(node ast.Node) {
		m := matcher{values: map[string]ast.Node{}}
		if m.node(exprNode, node) {
			matches = append(matches, node)
		}
	}
	ast.Inspect(node, func(node ast.Node) bool {
		match(node)
		for _, list := range exprLists(node) {
			match(list)
		}
		return true
	})
	return matches
}
