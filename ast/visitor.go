package ast

import (
	"reflect"
)

type Visitor interface {
	EnterScope()
	ExitScope()

	Visit(*Node) bool
	PostVisit(*Node)
}

type ASTVisitor struct {
	Visitor Visitor
}

func NewASTVisitor(visitor Visitor) *ASTVisitor {
	return &ASTVisitor{Visitor: visitor}
}

func (v *ASTVisitor) VisitSubmodule(submodule *Submodule) {
	v.EnterScope()
	submodule.Nodes = v.VisitNodes(submodule.Nodes)
	v.ExitScope()
}

func (v *ASTVisitor) VisitNodes(nodes []Node) []Node {
	res := make([]Node, len(nodes))
	for idx, node := range nodes {
		res[idx] = v.Visit(node)
	}
	return res
}

func (v *ASTVisitor) VisitExprs(exprs []Expr) []Expr {
	res := make([]Expr, len(exprs))
	for idx, expr := range exprs {
		res[idx] = v.VisitExpr(expr)
	}
	return res
}

func (v *ASTVisitor) VisitBlocks(blocks []*Block) []*Block {
	res := make([]*Block, len(blocks))
	for idx, block := range blocks {
		res[idx] = v.VisitBlock(block)
	}
	return res
}

func (v *ASTVisitor) Visit(n Node) Node {
	if isNil(n) {
		return nil
	}

	if v.Visitor.Visit(&n) {
		v.VisitChildren(n)
		v.Visitor.PostVisit(&n)
	}

	return n
}

func (v *ASTVisitor) VisitExpr(e Expr) Expr {
	if isNil(e) {
		return nil
	}

	n := e.(Node)
	if v.Visitor.Visit(&n) {
		v.VisitChildren(n)
		v.Visitor.PostVisit(&n)
	}

	if n != nil {
		return n.(Expr)
	} else {
		return nil
	}
}

func (v *ASTVisitor) VisitBlock(b *Block) *Block {
	if isNil(b) {
		return nil
	}

	n := Node(b)
	if v.Visitor.Visit(&n) {
		v.VisitChildren(n)
		v.Visitor.PostVisit(&n)
	}

	if n != nil {
		return n.(*Block)
	} else {
		return nil
	}
}

func (v *ASTVisitor) EnterScope() {
	v.Visitor.EnterScope()
}

func (v *ASTVisitor) ExitScope() {
	v.Visitor.ExitScope()
}

func (v *ASTVisitor) VisitChildren(n Node) {
	switch n := n.(type) {
	case *Block:
		if !n.NonScoping {
			v.EnterScope()
		}

		n.Nodes = v.VisitNodes(n.Nodes)

		if !n.NonScoping {
			v.ExitScope()
		}

	case *ReturnStat:
		n.Value = v.VisitExpr(n.Value)

	case *IfStat:
		n.Exprs = v.VisitExprs(n.Exprs)
		n.Bodies = v.VisitBlocks(n.Bodies)
		n.Else = v.VisitBlock(n.Else)

	case *AssignStat:
		n.Assignment = v.VisitExpr(n.Assignment)
		n.Access = v.Visit(n.Access).(AccessExpr)

	case *BinopAssignStat:
		n.Assignment = v.VisitExpr(n.Assignment)
		n.Access = v.Visit(n.Access).(AccessExpr)

	case *DestructAssignStat:
		n.Assignment = v.VisitExpr(n.Assignment)
		for idx, acc := range n.Accesses {
			n.Accesses[idx] = v.Visit(acc).(AccessExpr)
		}

	case *DestructBinopAssignStat:
		n.Assignment = v.VisitExpr(n.Assignment)
		for idx, acc := range n.Accesses {
			n.Accesses[idx] = v.Visit(acc).(AccessExpr)
		}

	case *LoopStat:
		n.Body = v.Visit(n.Body).(*Block)

		switch n.LoopType {
		case LOOP_TYPE_INFINITE:
		case LOOP_TYPE_CONDITIONAL:
			n.Condition = v.VisitExpr(n.Condition)
		default:
			panic("invalid loop type")
		}

	case *MatchStat:
		n.Target = v.VisitExpr(n.Target)

		res := make(map[Expr]Node)
		for pattern, stmt := range n.Branches {
			res[v.VisitExpr(pattern)] = v.Visit(stmt)
		}
		n.Branches = res

	case *BinaryExpr:
		n.Lhand = v.VisitExpr(n.Lhand)
		n.Rhand = v.VisitExpr(n.Rhand)

	case *CallExpr:
		n.Function = v.VisitExpr(n.Function)

		n.Arguments = v.VisitExprs(n.Arguments)
		n.ReceiverAccess = v.VisitExpr(n.ReceiverAccess)

	case *ArrayAccessExpr:
		n.Array = v.Visit(n.Array).(AccessExpr)
		n.Subscript = v.VisitExpr(n.Subscript)

	case *SizeofExpr:
		n.Expr = v.VisitExpr(n.Expr)

	case *ArrayLenExpr:
		n.Expr = v.VisitExpr(n.Expr)

	case *TupleLiteral:
		n.Members = v.VisitExprs(n.Members)

	case *CompositeLiteral:
		n.Values = v.VisitExprs(n.Values)

	case *EnumLiteral:
		n1 := v.Visit(n.TupleLiteral)
		if n1 == nil {
			n.TupleLiteral = nil
		} else {
			n.TupleLiteral = n1.(*TupleLiteral)
		}

		n2 := v.Visit(n.CompositeLiteral)
		if n2 == nil {
			n.CompositeLiteral = nil
		} else {
			n.CompositeLiteral = n2.(*CompositeLiteral)
		}

	case *BlockStat:
		n.Block = v.VisitBlock(n.Block)

	case *CallStat:
		n.Call = v.Visit(n.Call).(*CallExpr)

	case *DeferStat:
		n.Call = v.Visit(n.Call).(*CallExpr)

	case *ReferenceToExpr:
		n.Access = v.VisitExpr(n.Access)

	case *PointerToExpr:
		n.Access = v.VisitExpr(n.Access)

	case *CastExpr:
		n.Expr = v.VisitExpr(n.Expr)

	case *LambdaExpr:
		v.VisitFunction(n.Function)

	case *UnaryExpr:
		n.Expr = v.VisitExpr(n.Expr)

	case *StructAccessExpr:
		n.Struct = v.Visit(n.Struct).(AccessExpr)

	case *DerefAccessExpr:
		n.Expr = v.VisitExpr(n.Expr)

	case *FunctionDecl:
		v.VisitFunction(n.Function)

	case *VariableDecl:
		n.Assignment = v.VisitExpr(n.Assignment)

	case *DestructVarDecl:
		n.Assignment = v.VisitExpr(n.Assignment)

	case *FunctionAccessExpr:
		n.ReceiverAccess = v.VisitExpr(n.ReceiverAccess)

	case *NumericLiteral, *StringLiteral, *BoolLiteral, *RuneLiteral,
		*VariableAccessExpr, *TypeDecl, *UseDirective, *BreakStat, *NextStat,
		*DiscardAccessExpr, *EnumPatternExpr:
		// do nothing

	default:
		panic("Unhandled node: " + reflect.TypeOf(n).String())
	}
}

func (v *ASTVisitor) VisitFunction(fn *Function) {
	v.EnterScope()

	if fn.Type.Receiver != nil {
		fn.Receiver = v.Visit(fn.Receiver).(*VariableDecl)
	}

	for idx, param := range fn.Parameters {
		fn.Parameters[idx] = v.Visit(param).(*VariableDecl)
	}

	if fn.Body != nil {
		fn.Body = v.Visit(fn.Body).(*Block)
	}
	v.ExitScope()
}

// IterNodes allows for looping over all nodes using a for range loop
func (v *Submodule) IterNodes() chan Node {
	cv := &channelingVisitor{
		channel: make(chan Node),
	}
	go cv.run(v)
	return cv.channel
}

type channelingVisitor struct {
	channel chan Node
}

func (v *channelingVisitor) run(submod *Submodule) {
	vis := NewASTVisitor(v)
	vis.VisitSubmodule(submod)
	close(v.channel)
}

func (v *channelingVisitor) Visit(n *Node) bool {
	v.channel <- *n
	return true
}

func (v *channelingVisitor) EnterScope()       {}
func (v *channelingVisitor) ExitScope()        {}
func (v *channelingVisitor) PostVisit(n *Node) {}

func isNil(a interface{}) bool {
	defer func() { recover() }()
	return a == nil || reflect.ValueOf(a).IsNil()
}
