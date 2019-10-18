package semantic

import (
	"github.com/ku-lang/ku/ast"
	"github.com/ku-lang/ku/util"
)

// TODO handle match/switch, if we need to

type BreakAndContinueCheck struct {
	nestedLoopCount map[*ast.Function]int
	functions       []*ast.Function
}

func (_ BreakAndContinueCheck) Name() string { return "break and next" }

func (v *BreakAndContinueCheck) Init(s *SemanticAnalyzer) {
	v.nestedLoopCount = make(map[*ast.Function]int)
	v.functions = nil
}

func (v *BreakAndContinueCheck) EnterScope(s *SemanticAnalyzer) {}
func (v *BreakAndContinueCheck) ExitScope(s *SemanticAnalyzer)  {}
func (v *BreakAndContinueCheck) Finalize(s *SemanticAnalyzer)   {}

func (v *BreakAndContinueCheck) Visit(s *SemanticAnalyzer, n ast.Node) {
	switch n := n.(type) {
	case *ast.ContinueStat, *ast.BreakStat:
		if v.nestedLoopCount[v.functions[len(v.functions)-1]] == 0 {
			s.Err(n, "%s must be in a loop", util.CapitalizeFirst(n.NodeName()))
		}

	case *ast.LoopStat:
		v.nestedLoopCount[v.functions[len(v.functions)-1]]++

	case *ast.FunctionDecl:
		v.functions = append(v.functions, n.Function)
	case *ast.LambdaExpr:
		v.functions = append(v.functions, n.Function)
	}
}

func (v *BreakAndContinueCheck) PostVisit(s *SemanticAnalyzer, n ast.Node) {
	switch n := n.(type) {
	case *ast.Block:
		for i, c := range n.Nodes {
			if i < len(n.Nodes)-1 && isBreakOrContinue(c) {
				s.Err(n.Nodes[i+1], "Unreachable code")
			}
		}

	case *ast.LoopStat:
		v.nestedLoopCount[v.functions[len(v.functions)-1]]--
	case *ast.FunctionDecl:
		v.functions = v.functions[:len(v.functions)-1]
		delete(v.nestedLoopCount, n.Function)
	case *ast.LambdaExpr:
		v.functions = v.functions[:len(v.functions)-1]
		delete(v.nestedLoopCount, n.Function)
	}
}

func isBreakOrContinue(n ast.Node) bool {
	switch n.(type) {
	case *ast.BreakStat, *ast.ContinueStat:
		return true
	}
	return false
}
