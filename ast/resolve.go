package ast

import (
	"fmt"
	"os"
	"reflect"

	"github.com/ku-lang/ku/util"
	"github.com/ku-lang/ku/util/log"
)

type UnresolvedName struct {
	ModuleNames []string
	Name        string
}

func (v UnresolvedName) String() string {
	ret := ""
	for _, mod := range v.ModuleNames {
		//ret += mod + "::"
		ret += mod + "."
	}
	return ret + v.Name
}

func (v UnresolvedName) Split() (UnresolvedName, string) {
	if len(v.ModuleNames) > 0 {
		res := UnresolvedName{}
		res.ModuleNames = v.ModuleNames[:len(v.ModuleNames)-1]
		res.Name = v.ModuleNames[len(v.ModuleNames)-1]
		return res, v.Name
	} else {
		return UnresolvedName{}, ""
	}
}

type Resolver struct {
	modules       *ModuleLookup
	module        *Module
	cModule       *Module
	curSubmod     *Submodule
	functionStack []*Function
	curScope      *Scope
}

func (v *Resolver) pushFunction(fn *Function) {
	v.functionStack = append(v.functionStack, fn)
}

func (v *Resolver) popFunction() {
	v.functionStack = v.functionStack[:len(v.functionStack)-1]
}

func (v Resolver) currentFunction() *Function {
	if len(v.functionStack) == 0 {
		return nil
	}
	return v.functionStack[len(v.functionStack)-1]
}

func Resolve(mod *Module, mods *ModuleLookup) {
	if mod.resolved {
		return
	}
	mod.resolved = true

	res := &Resolver{
		modules: mods,
		module:  mod,
		cModule: &Module{
			Name:    &ModuleName{Parts: []string{"C"}},
			Parts:   make(map[string]*Submodule),
			Dirpath: "", // not really a path for this module
		},
	}

	res.cModule.ModScope = NewCScope(res.cModule)

	res.curScope = NewGlobalScope(mod)
	mod.ModScope = res.curScope

	// add a C module here which will contain all of the c bindings and what
	// not to keep everything separate
	mod.ModScope.UsedModules["C"] = res.cModule

	res.ResolveUsedModules()
	log.Timed("resolving module", mod.Name.String(), func() {
		res.ResolveTopLevelDecls()
		res.ResolveDescent()
	})
	res.module.ModScope.Dump(0)
}

func (v *Resolver) ResolveUsedModules() {
	for _, submod := range v.module.Parts {
		submod.UseScope = newScope(nil, v.module, nil)

		for _, node := range submod.Nodes {
			switch node := node.(type) {
			case *UseDirective:
				// TODO: Propagate this down into the parser/constructor
				modName := ModuleNameFromUnresolvedName(node.ModuleName)
				usedMod, err := v.modules.Get(modName)
				if err == nil {
					Resolve(usedMod.Module, v.modules)
				} else {
					panic("INTERNAL ERROR: Used module not loaded")
				}
				submod.UseScope.UseModule(usedMod.Module)

			default:
				continue
			}
		}
	}
}

func (v *Resolver) ResolveTopLevelDecls() {
	modScope := v.module.ModScope

	var staticFuncList []*FunctionDecl

	for _, submod := range v.module.Parts {
		for _, node := range submod.Nodes {
			switch node := node.(type) {
			// TODO: We might need to do more that just insert this into the
			// scope at the current point.
			case *TypeDecl:
				if modScope.InsertType(node.NamedType, node.IsPublic()) != nil {
					v.err(node, "Illegal redeclaration of type `%s`", node.NamedType.Name)
				}

			case *FunctionDecl:
				if node.Function.Receiver == nil {
					if node.Function.StaticReceiverType == nil {
						scope := v.curScope
						if node.Function.Type.Attrs().Contains("C") {
							scope = v.cModule.ModScope
							node.SetPublic(true)
						}

						if scope.InsertFunction(node.Function, node.IsPublic()) != nil {
							v.err(node, "Illegal redeclaration of function `%s`", node.Function.Name)
						}
					} else {
						staticFuncList = append(staticFuncList, node)
					}
				}

			case *VariableDecl:
				if modScope.InsertVariable(node.Variable, node.IsPublic()) != nil {
					v.err(node, "Illegal redeclaration of variable `%s`", node.Variable.Name)
				}

			default:
				continue
			}
		}
	}

	for _, node := range staticFuncList {
		node.Function.StaticReceiverType = v.ResolveType(node, node.Function.StaticReceiverType)
		if checkReceiverType(v, node, &TypeReference{BaseType: node.Function.StaticReceiverType}, "static receiver") {
			node.Function.StaticReceiverType.(*NamedType).addStaticMethod(node.Function)
		}
	}
}

func (v *Resolver) ResolveDescent() {
	vis := NewASTVisitor(v)
	for _, submod := range v.module.Parts {
		v.curSubmod = submod
		vis.VisitSubmodule(submod)
	}
}

func (v *Resolver) err(thing Locatable, err string, stuff ...interface{}) {
	pos := thing.Pos()

	log.Error("resolve", util.TEXT_RED+util.TEXT_BOLD+"error:"+util.TEXT_RESET+" [%s:%d:%d] %s\n",
		pos.Filename, pos.Line, pos.Char, fmt.Sprintf(err, stuff...))

	if v.curSubmod != nil {
		log.Error("resolve", v.curSubmod.File.MarkPos(pos))
	}

	os.Exit(util.EXIT_FAILURE_SEMANTIC)
}

func (v *Resolver) tryGetIdent(loc Locatable, name UnresolvedName) *Ident {
	// TODO: Decide whether we should actually allow shadowing a module
	//fmt.Printf("[CurScope]:%#v\n", v.curScope)
	ident := v.curScope.GetIdent(name)
	if ident == nil {
		ident = v.curSubmod.UseScope.GetIdent(name)
	}

	if ident == nil {
		log.Errorln("resolve", "Cannot resolve `%s`", name.String())
		return nil
	}

	if !ident.Public && ident.Scope.Module != v.module {
		log.Errorln("resolve", "Cannot access private identifier `%s`", name)
	}

	// make sure lambda can't access variables of enclosing function
	if ident.Scope.Function != nil && v.currentFunction() != ident.Scope.Function {
		log.Errorln("resolve", "Cannot access local identifier `%s` from lambda", name)
	}

	return ident
}

func (v *Resolver) getIdent(loc Locatable, name UnresolvedName) *Ident {
	// TODO: Decide whether we should actually allow shadowing a module
	//fmt.Printf("[CurScope]:%#v\n", v.curScope)
	ident := v.curScope.GetIdent(name)
	if ident == nil {
		ident = v.curSubmod.UseScope.GetIdent(name)
	}

	if ident == nil {
		v.err(loc, "Cannot resolve `%s`", name.String())
		return nil
	}

	if !ident.Public && ident.Scope.Module != v.module {
		v.err(loc, "Cannot access private identifier `%s`", name)
	}

	// make sure lambda can't access variables of enclosing function
	if ident.Scope.Function != nil && v.currentFunction() != ident.Scope.Function {
		v.err(loc, "Cannot access local identifier `%s` from lambda", name)
	}

	return ident
}

func (v *Resolver) Visit(n *Node) bool {
	v.ResolveNode(n)
	return true
}

func (v *Resolver) PostVisit(node *Node) {
	switch n := (*node).(type) {
	case *FunctionDecl:
		// Store the method in the type of the reciever
		if n.Function.Type.Receiver != nil {
			if named, ok := TypeWithoutPointers(n.Function.Receiver.Variable.Type.BaseType).(*NamedType); ok {
				named.addMethod(n.Function)
			}
		}

		v.ExitScope()
		v.popFunction()

	case *LambdaExpr:
		v.popFunction()
	}
}

func (v *Resolver) EnterScope() {
	v.curScope = newScope(v.curScope, v.module, v.currentFunction())
}

func (v *Resolver) ExitScope() {
	if v.curScope.Outer == nil {
		panic("INTERNAL ERROR: Trying to exit highest scope")
	}
	v.curScope = v.curScope.Outer
}

// returns true if no error
func checkReceiverType(res *Resolver, loc Locatable, t *TypeReference, purpose string) bool {
	if named, ok := TypeReferenceWithoutPointers(t).BaseType.(*NamedType); ok {
		if named.ParentModule != res.module {
			res.err(loc, "Cannot use type `%s` declared in module `%s` as %s",
				t.String(), named.ParentModule.Name, purpose)
			return false
		}
	} else {
		res.err(loc, "Expected named type for %s, found `%s`", purpose, t.String())
		return false
	}
	return true
}

func (v *Resolver) ResolveNode(node *Node) {
	switch n := (*node).(type) {
	case *TypeDecl:
		// Only resolve non-generic type, generic types will currently be
		// resolved when they are used, as the type parameters can only be
		// resolved when we know what they are.
		n.NamedType.Type = v.ResolveType(n, n.NamedType.Type)

	case *FunctionDecl:
		v.EnterScope()
		v.pushFunction(n.Function)

		// 将this变量插入到当前scope中
		if n.Function.Receiver != nil {
			if v.curScope.InsertVariable(n.Function.Receiver.Variable, false) != nil {
				v.err(n, "Illegal redeclaration of variable `%s`", n.Function.Receiver.Variable.Name)
			}
		}

		for _, par := range n.Function.Type.GenericParameters {
			if v.curScope.InsertType(par, false) != nil {
				v.err(n, "Illegal redeclaration of generic type parameter `%s`", par.TypeName())
			}
		}

		n.Function.Type = v.ResolveType(n, n.Function.Type).(FunctionType)

	case *VariableDecl:
		if n.Variable.Type != nil {
			n.Variable.Type = v.ResolveTypeReference(n, n.Variable.Type)
		}
		if v.curScope.InsertVariable(n.Variable, n.IsPublic()) != nil {
			v.err(n, "Illegal redeclaration of variable `%s`", n.Variable.Name)
		}

	case *DestructVarDecl:
		for idx, vari := range n.Variables {
			if !n.ShouldDiscard[idx] && v.curScope.InsertVariable(vari, false) != nil {
				v.err(n, "Illegal redeclaration of variable `%s`", vari.Name)
			}
		}

	// Expr

	case *LambdaExpr:
		v.pushFunction(n.Function)

		n.Function.Type = v.ResolveType(n, n.Function.Type).(FunctionType)

	case *CastExpr:
		n.Type = v.ResolveTypeReference(n, n.Type)

	case *ArrayLenExpr:
		if n.Type != nil {
			n.Type = v.ResolveType(n, n.Type)
		}

	case *EnumLiteral:
		n.Type = v.ResolveTypeReference(n, n.Type)

	case *VariableAccessExpr:

		// TODO: Check if we can clean this up
		// NOTE: Here we check whether this is actually a variable access or an enum member.
		//fmt.Printf("vaexpr:%#v\n", n.Name)
		if len(n.Name.ModuleNames) > 0 {
			enumName, memberName := n.Name.Split()
			ident := v.tryGetIdent(n, enumName)
			if ident != nil && ident.Type == IDENT_TYPE {
				itype := ident.Value.(Type)
				if etype, ok := itype.ActualType().(EnumType); ok {
					if _, ok := etype.GetMember(memberName); !ok {
						v.err(n, "No such member in enum `%s`: `%s`", itype.TypeName(), memberName)
						break
					}

					enum := &EnumLiteral{}
					enum.Member = memberName
					enum.Type = &TypeReference{
						BaseType: UnresolvedType{
							Name: enumName,
						},
						GenericArguments: v.ResolveTypeReferences(n, n.GenericArguments),
					}
					enum.Type = v.ResolveTypeReference(n, enum.Type)
					enum.SetPos(n.Pos())

					*node = enum
					break
				}
			}
		}

		var memberName string
		//fmt.Printf("[try name 1]: %#v\n", n.Name)

		ident := v.tryGetIdent(n, n.Name)
		var wrap *StructAccessExpr
		//fmt.Printf("[try name]: %#v\n", n.Name)
		for ident == nil && len(n.Name.ModuleNames) > 0 {
			log.Debugln("resolve", "trying to resolve VariableAccessNode as StructAccessNode: %#v", n)
			// 如果名字获取不到，说明有可能实际是StructAccess，尝试向前移动一个词，重新检验
			var parentName UnresolvedName
			parentName, memberName = n.Name.Split()
			n.Name = parentName
			log.Debugln("resolve", "new name: %#v; member: %#v", parentName, memberName)
			ident = v.tryGetIdent(n, parentName)
			log.Debugln("resolve", "ident: %#v", ident)

			sae := &StructAccessExpr{
				Member:         memberName,
				Struct:         n,
				ParentFunction: v.currentFunction(),
			}
			if wrap == nil {
				wrap = sae
			} else {
				wrap.Struct = sae
			}

		}
		if wrap != nil {
			*node = wrap
			(*node).SetPos(n.Pos())
		}
		log.Debugln("resolve", "VariableAccessExpr:%#v", *node)

		if ident == nil {
			v.err(n, "Cannot resolve ident `%s`", n.Name.String())
		}

		if ident.Type == IDENT_FUNCTION {
			fan := &FunctionAccessExpr{
				Function:         ident.Value.(*Function),
				GenericArguments: v.ResolveTypeReferences(n, n.GenericArguments),
				ParentFunction:   v.currentFunction(),
			}
			fan.Function.Accesses = append(fan.Function.Accesses, fan)
			*node = fan
			(*node).SetPos(n.Pos())
			break
		} else if ident.Type == IDENT_VARIABLE {
			n.Variable = ident.Value.(*Variable)
		} else {
			v.err(n, "Expected variable identifier, found %s `%s`", ident.Type, n.Name)
		}

		if n.Variable != nil && n.Variable.Type != nil {
			n.Variable.Type = v.ResolveTypeReference(n, n.Variable.Type)
		}

	case *SizeofExpr:
		if n.Expr != nil {
			if typ, ok := v.exprToType(n.Expr); ok {
				n.Expr = nil
				n.Type = &TypeReference{BaseType: typ}
			}
		}

		if n.Type != nil {
			n.Type = v.ResolveTypeReference(n, n.Type)
		}

	case *CompositeLiteral:
		if n.Type == nil {
			break
		}

		// NOTE: Here we check if we are referencing an actual struct,
		// or the struct part of an enum type
		if name, ok := n.Type.BaseType.(UnresolvedType); ok {
			enumName, memberName := name.Name.Split()
			if memberName != "" {
				ident := v.getIdent(n, enumName)
				if ident.Type == IDENT_TYPE {
					itype := ident.Value.(Type)
					if _, ok := itype.ActualType().(EnumType); ok {
						et := v.ResolveTypeReference(n, &TypeReference{
							BaseType: UnresolvedType{
								Name: enumName,
							},
							GenericArguments: v.ResolveTypeReferences(n, n.Type.GenericArguments),
						})

						member, ok := et.BaseType.ActualType().(EnumType).GetMember(memberName)
						if !ok {
							v.err(n, "Enum `%s` has no member `%s`", enumName.String(), memberName)
						}

						enum := &EnumLiteral{}
						enum.Member = memberName
						enum.Type = &TypeReference{BaseType: itype, GenericArguments: et.GenericArguments}
						enum.CompositeLiteral = n
						enum.CompositeLiteral.Type = &TypeReference{BaseType: member.Type, GenericArguments: et.GenericArguments}
						enum.SetPos(n.Pos())

						*node = enum
						break
					}
				}
			}
		}

		if n.Type != nil {
			n.Type = v.ResolveTypeReference(n, n.Type)

			var gcon *GenericContext
			if len(n.Type.GenericArguments) > 0 {
				gcon = NewGenericContextFromTypeReference(n.Type)
			}

			// We do some preliminary type hinting to help out the inferrence pass
			if at, ok := n.Type.BaseType.(ArrayType); ok {
				for _, val := range n.Values {
					if gcon != nil {
						val.SetType(gcon.Replace(at.MemberType))
					} else {
						val.SetType(at.MemberType)
					}
				}
			} else if st, ok := n.Type.BaseType.(StructType); ok {
				for idx, val := range n.Values {
					field := n.Fields[idx]
					mem := st.GetMember(field)
					if gcon != nil {
						val.SetType(gcon.Replace(mem.Type))
					} else {
						val.SetType(mem.Type)
					}
				}
			}

			switch n.Type.BaseType.ActualType().(type) {
			case StructType, ArrayType:

			default:
				v.err(n, "Type `%s` is not composite type", n.Type.String())
			}
		}

	case *CallExpr:
		log.Debugln("resolve", "checking callexpr:%#v", n.Function)
		log.Debugln("resolve", "checking callexpr receiver:%#v", n.ReceiverAccess)

		// NOTE: Here we check whether this is a call or an enum tuple lit.
		// way too much duplication with all this enum literal creating stuff
		if vae, ok := n.Function.(*VariableAccessExpr); ok {

			if len(vae.Name.ModuleNames) > 0 {
				enumName, memberName := vae.Name.Split()
				ident := v.tryGetIdent(n, enumName)
				if ident != nil && ident.Type == IDENT_TYPE {
					itype := ident.Value.(Type)
					if _, ok := itype.ActualType().(EnumType); ok {
						et := v.ResolveTypeReference(n, &TypeReference{
							BaseType: UnresolvedType{
								Name: enumName,
							},
							GenericArguments: v.ResolveTypeReferences(vae, vae.GenericArguments),
						})

						member, ok := et.BaseType.ActualType().(EnumType).GetMember(memberName)
						if !ok {
							v.err(n, "Enum `%s` has no member `%s`", enumName.String(), memberName)
						}

						enum := &EnumLiteral{}
						enum.Member = memberName
						enum.Type = et
						enum.TupleLiteral = &TupleLiteral{
							Members:           n.Arguments,
							Type:              &TypeReference{BaseType: member.Type, GenericArguments: et.GenericArguments},
							ParentEnumLiteral: enum,
						}
						enum.TupleLiteral.SetPos(n.Pos())
						enum.SetPos(n.Pos())

						*node = enum
						break
					}
				}
			}

			ident := v.tryGetIdent(n, vae.Name)
			var wrap *StructAccessExpr
			for ident == nil && len(vae.Name.ModuleNames) > 0 {
				log.Debugln("resolve", "trying to resolve VariableAccessNode as StructAccessNode: %#v", n)
				// 如果名字获取不到，说明有可能实际是StructAccess，尝试向前移动一个词，重新检验
				parentName, memberName := vae.Name.Split()
				vae.Name = parentName
				log.Debugln("resolve", "new name: %#v; member: %#v", parentName, memberName)
				ident = v.tryGetIdent(n, parentName)
				log.Debugln("resolve", "ident: %#v", ident)
				sae := &StructAccessExpr{
					Member:         memberName,
					Struct:         vae,
					ParentFunction: v.currentFunction(),
				}
				if wrap == nil {
					wrap = sae
				} else {
					wrap.Struct = sae
				}

				log.Debugln("resolve", "got strctAccessExpr:%#v", wrap)
			}
			if wrap != nil {
				n.Function = wrap
				n.ReceiverAccess = vae
			}

			log.Debugln("resolve", "checking callexpr:%#v", n.Function)
			log.Debugln("resolve", "checking callexpr receiver:%#v", n.ReceiverAccess)
		}

		// NOTE: Here we check whether this is a call or a cast
		// Unwrap any deref access expressions as these might signify pointer types
		if typ, ok := v.exprToType(n.Function); ok {
			if len(n.Arguments) != 1 {
				v.err(n, "Casts must recieve exactly one argument")
			}

			cast := &CastExpr{}
			cast.Type = &TypeReference{BaseType: typ}
			cast.Expr = n.Arguments[0]
			cast.SetPos(n.Pos())
			*node = cast
		}

	case *StructAccessExpr:
		n.ParentFunction = v.currentFunction()

	case *EnumPatternExpr:
		for _, vari := range n.Variables {
			if vari != nil && v.curScope.InsertVariable(vari, false) != nil {
				v.err(n, "Illegal redeclaration of variable `%s`", vari.Name)
			}
		}

	// No-Ops
	case *Block, *UseDirective, *AssignStat, *BinopAssignStat,
		*DestructAssignStat, *DestructBinopAssignStat, *BlockStat, *BreakStat,
		*CallStat, *DeferStat, *IfStat, *MatchStat, *LoopStat, *ContinueStat,
		*ReturnStat, *ReferenceToExpr, *PointerToExpr, *ArrayAccessExpr,
		*BinaryExpr, *DerefAccessExpr, *UnaryExpr, *DiscardAccessExpr, *BoolLiteral,
		*NumericLiteral, *RuneLiteral, *StringLiteral, *TupleLiteral:
		break

	default:
		panic("INTERNAL ERROR: Unhandled node in resolve pass `" + reflect.TypeOf(n).String() + "`")
	}
}

func (v *Resolver) exprToType(expr Expr) (Type, bool) {
	var references []bool
	var mutable []bool
	for {
		if rte, ok := expr.(*ReferenceToExpr); ok {
			references = append(references, true)
			mutable = append(mutable, rte.IsMutable)
			expr = rte.Access
		} else if pte, ok := expr.(*PointerToExpr); ok {
			references = append(references, false)
			mutable = append(mutable, pte.IsMutable)
			expr = pte.Access
		} else {
			break
		}
	}

	if vae, ok := expr.(*VariableAccessExpr); ok {
		ident := v.getIdent(vae, vae.Name)
		if ident != nil && ident.Type == IDENT_TYPE {
			res := ident.Value.(Type)
			for idx, isReference := range references {
				isMutable := mutable[idx]
				if isReference {
					res = ReferenceTo(&TypeReference{BaseType: res}, isMutable)
				} else {
					res = PointerTo(&TypeReference{BaseType: res}, isMutable)
				}
			}
			return res, true
		}
	}

	return nil, false
}

func (v *Resolver) ResolveTypes(src Locatable, ts []Type) []Type {
	res := make([]Type, 0, len(ts))
	for _, t := range ts {
		res = append(res, v.ResolveType(src, t))
	}
	return res
}

func (v *Resolver) ResolveTypeReferences(src Locatable, ts []*TypeReference) []*TypeReference {
	res := make([]*TypeReference, 0, len(ts))
	for _, t := range ts {
		res = append(res, v.ResolveTypeReference(src, t))
	}
	return res
}

func (v *Resolver) ResolveTypeReference(src Locatable, t *TypeReference) *TypeReference {
	return &TypeReference{
		BaseType:         v.ResolveType(src, t.BaseType),
		GenericArguments: v.ResolveTypeReferences(src, t.GenericArguments),
	}
}

func (v *Resolver) ResolveType(src Locatable, t Type) Type {
	switch t := t.(type) {
	case PrimitiveType, *NamedType:
		return t

	case InterfaceType:
		v.EnterScope()

		for _, gpar := range t.GenericParameters {
			v.curScope.InsertType(gpar, false)
		}

		for _, fn := range t.Functions {
			fn.Type = v.ResolveType(src, fn.Type).(FunctionType)

			if fn.StaticReceiverType != nil {
				fn.StaticReceiverType = v.ResolveType(src, fn.StaticReceiverType)
			}
		}

		v.ExitScope()

		return &InterfaceType{
			Functions:         t.Functions,
			attrs:             t.attrs,
			GenericParameters: t.GenericParameters,
		}

	case ArrayType:
		return ArrayOf(v.ResolveTypeReference(src, t.MemberType), t.IsFixedLength, t.Length)

	case ReferenceType:
		return ReferenceTo(v.ResolveTypeReference(src, t.Referrer), t.IsMutable)

	case PointerType:
		return PointerTo(v.ResolveTypeReference(src, t.Addressee), t.IsMutable)

	case *SubstitutionType:
		var constraints []*TypeReference
		for _, c := range t.Constraints {
			rc := v.ResolveTypeReference(src, c)

			if _, ok := rc.BaseType.ActualType().(InterfaceType); !ok {
				v.err(src, "Generic parameter constraint must be interface")
			}

			constraints = append(constraints, rc)
		}
		t.Constraints = constraints
		return t

	case StructType:
		v.EnterScope()

		for _, gpar := range t.GenericParameters {
			v.curScope.InsertType(gpar, false)
		}

		nt := StructType{
			Module:            t.Module,
			Members:           make([]*StructMember, len(t.Members)),
			attrs:             t.attrs,
			GenericParameters: t.GenericParameters,
		}

		v.EnterScope()
		for idx, mem := range t.Members {
			nt.Members[idx] = &StructMember{
				Name:   mem.Name,
				Type:   v.ResolveTypeReference(src, mem.Type),
				Public: mem.Public,
			}
		}
		v.ExitScope()

		v.ExitScope()

		return nt

	case TupleType:
		nt := TupleType{Members: make([]*TypeReference, len(t.Members))}

		for idx, mem := range t.Members {
			nt.Members[idx] = v.ResolveTypeReference(src, mem)
		}

		return nt

	case EnumType:
		v.EnterScope()

		for _, gpar := range t.GenericParameters {
			v.curScope.InsertType(gpar, false)
		}

		nv := EnumType{
			Simple:            t.Simple,
			Members:           make([]EnumTypeMember, len(t.Members)),
			attrs:             t.attrs,
			GenericParameters: t.GenericParameters,
		}

		for idx, mem := range t.Members {
			nv.Members[idx].Name = mem.Name
			nv.Members[idx].Tag = mem.Tag
			nv.Members[idx].Type = v.ResolveType(src, mem.Type)
		}

		v.ExitScope()

		return nv

	case FunctionType:
		nv := FunctionType{
			attrs:             t.attrs,
			IsVariadic:        t.IsVariadic,
			Parameters:        v.ResolveTypeReferences(src, t.Parameters),
			GenericParameters: t.GenericParameters,
		}

		if t.Receiver != nil {
			nv.Receiver = v.ResolveTypeReference(src, t.Receiver)
			checkReceiverType(v, src, nv.Receiver, "receiver")
		}
		if t.Return != nil {
			nv.Return = v.ResolveTypeReference(src, t.Return)
		}

		return nv

	case UnresolvedType:
		ident := v.getIdent(src, t.Name)
		if ident == nil {
			// do nothing
		} else if ident.Type != IDENT_TYPE {
			v.err(src, "Expected type identifier, found %s `%s`", ident.Type, t.Name)
		} else {
			return v.ResolveType(src, ident.Value.(Type))
		}

		panic("unreachable")

	default:
		typeName := reflect.TypeOf(t).String()
		panic("INTERNAL ERROR: Unhandled type in resolve pass: " + typeName)
	}
}
