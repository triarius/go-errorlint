package errorlint

import (
	"flag"
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/analysis"
)

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:  "errorlint",
		Doc:   "Source code linter for Go software that can be used to find code that will cause problems with the error wrapping scheme introduced in Go 1.13.",
		Run:   run,
		Flags: flagSet,
	}
}

var (
	flagSet          flag.FlagSet
	checkComparison  bool
	checkAsserts     bool
	checkErrorf      bool
	checkErrorfMulti bool
)

func init() {
	flagSet.BoolVar(&checkComparison, "comparison", true, "Check for plain error comparisons")
	flagSet.BoolVar(&checkAsserts, "asserts", true, "Check for plain type assertions and type switches")
	flagSet.BoolVar(&checkErrorf, "errorf", false, "Check whether fmt.Errorf uses the %w verb for formatting errors. See the readme for caveats")
	flagSet.BoolVar(&checkErrorfMulti, "errorf-multi", true, "Permit more than 1 %w verb, valid per Go 1.20 (Requires -errorf=true)")
}

func run(pass *analysis.Pass) (interface{}, error) {
	lints := []analysis.Diagnostic{}
	extInfo := newTypesInfoExt(pass)
	if checkComparison {
		l := LintErrorComparisons(extInfo)
		lints = append(lints, l...)
	}
	if checkAsserts {
		l := LintErrorTypeAssertions(pass.Fset, *pass.TypesInfo)
		lints = append(lints, l...)
	}
	if checkErrorf {
		l := LintFmtErrorfCalls(pass.Fset, *pass.TypesInfo, checkErrorfMulti)
		lints = append(lints, l...)
	}
	sort.Sort(ByPosition(lints))

	for _, l := range lints {
		pass.Report(l)
	}
	return nil, nil
}

type TypesInfoExt struct {
	*analysis.Pass

	// Maps AST nodes back to the node they are contained within.
	NodeParent map[ast.Node]ast.Node

	// Maps an object back to all identifiers to refer to it.
	IdentifiersForObject map[types.Object][]*ast.Ident
}

func newTypesInfoExt(pass *analysis.Pass) *TypesInfoExt {
	nodeParent := map[ast.Node]ast.Node{}
	for node := range pass.TypesInfo.Scopes {
		file, ok := node.(*ast.File)
		if !ok {
			continue
		}
		stack := []ast.Node{file}
		ast.Inspect(file, func(n ast.Node) bool {
			nodeParent[n] = stack[len(stack)-1]
			if n == nil {
				stack = stack[:len(stack)-1]
			} else {
				stack = append(stack, n)
			}
			return true
		})
	}

	identifiersForObject := map[types.Object][]*ast.Ident{}
	for node, obj := range pass.TypesInfo.Defs {
		identifiersForObject[obj] = append(identifiersForObject[obj], node)
	}
	for node, obj := range pass.TypesInfo.Uses {
		identifiersForObject[obj] = append(identifiersForObject[obj], node)
	}

	return &TypesInfoExt{
		Pass:                 pass,
		NodeParent:           nodeParent,
		IdentifiersForObject: identifiersForObject,
	}
}

func (info *TypesInfoExt) ContainingFuncDecl(node ast.Node) *ast.FuncDecl {
	for parent := info.NodeParent[node]; ; parent = info.NodeParent[parent] {
		if _, ok := parent.(*ast.File); ok {
			break
		}
		if fun, ok := parent.(*ast.FuncDecl); ok {
			return fun
		}
	}
	return nil
}
