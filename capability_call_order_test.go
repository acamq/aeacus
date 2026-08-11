package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFreeBSDShellValidationPrecedesPhocusShellSocket(t *testing.T) {
	configs := parseGoFileForCapabilityTest(t, "configs.go")
	parseConfigDecl := findFunctionForCapabilityTest(t, configs, "parseConfig")
	validatePosition := findCallForCapabilityTest(t, parseConfigDecl, "resolveRuntimeCapabilities")
	conditionPosition := findCallForCapabilityTest(t, parseConfigDecl, "validateConfigConditions")
	publishPosition := findAssignmentForCapabilityTest(t, parseConfigDecl, "conf")
	if validatePosition >= conditionPosition || conditionPosition >= publishPosition {
		t.Fatalf("validation/publication order = capability:%d condition:%d publish:%d", validatePosition, conditionPosition, publishPosition)
	}

	score := parseGoFileForCapabilityTest(t, "score.go")
	readScoringDataDecl := findFunctionForCapabilityTest(t, score, "readScoringData")
	findCallForCapabilityTest(t, readScoringDataDecl, "parseConfig")

	phocus := parseGoFileForCapabilityTest(t, "phocus.go")
	loopDecl := findFunctionForCapabilityTest(t, phocus, "phocusLoop")
	environmentPosition := findCallForCapabilityTest(t, loopDecl, "phocusEnvironment")
	shellPosition := findCallForCapabilityTest(t, loopDecl, "shellSocket")
	if environmentPosition >= shellPosition {
		t.Fatalf("phocus environment position %d must precede shell socket %d", environmentPosition, shellPosition)
	}
	environmentDecl := findFunctionForCapabilityTest(t, phocus, "phocusEnvironment")
	findCallForCapabilityTest(t, environmentDecl, "readScoringData")
}

func parseGoFileForCapabilityTest(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func findFunctionForCapabilityTest(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == name {
			return function
		}
	}
	t.Fatalf("function %s not found", name)
	return nil
}

func findCallForCapabilityTest(t *testing.T, function *ast.FuncDecl, name string) token.Pos {
	t.Helper()
	position := token.NoPos
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == name {
			position = call.Pos()
			return false
		}
		return true
	})
	if position == token.NoPos {
		t.Fatalf("call %s not found in %s", name, function.Name.Name)
	}
	return position
}

func findAssignmentForCapabilityTest(t *testing.T, function *ast.FuncDecl, name string) token.Pos {
	t.Helper()
	position := token.NoPos
	ast.Inspect(function.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, target := range assignment.Lhs {
			identifier, ok := target.(*ast.Ident)
			if ok && identifier.Name == name {
				position = assignment.Pos()
				return false
			}
		}
		return true
	})
	if position == token.NoPos {
		t.Fatalf("assignment to %s not found in %s", name, function.Name.Name)
	}
	return position
}
