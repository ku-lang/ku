package parser

// Note that you should include a lot of calls to panic() where something's happening that shouldn't be.
// This will help to find bugs. Once the compiler is in a better state, a lot of these calls can be removed.

import (
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/ku-lang/ku/util/log"

	"github.com/ku-lang/ku/lexer"
	"github.com/ku-lang/ku/util"
)

// parser 语法分析类，用于存放语法分析的环境
type parser struct {
	input        *lexer.Sourcefile // 输入，即词法分析的输出，包括源文件信息与Token列表
	currentToken int               // 当前Token：语法分析逐个分析Token列表，因此需要记录当前所前进到的Token
	tree         *ParseTree        // 分析结果：一个语法分析树

	binOpPrecedences  map[BinOpType]int // 二元操作符的优先读
	curNodeTokenStart int               // 当前节点的起始Token
	ruleStack         []string          // 规则堆栈，？？
	deps              []*NameNode       // 深度，？？
}

// Parse 语法分析的主功能函数，由main.go调用
// input 语法分析的输入是词法分析输出的一个Sourcefile对象，其中包括源文件以及所有的Token词号列表。
// 该函数返回一个语法分析树（ParseTree）实例，以及个名字节点的列表
func Parse(input *lexer.Sourcefile) (*ParseTree, []*NameNode) {
	p := &parser{
		input:            input,
		binOpPrecedences: newBinOpPrecedenceMap(),
		tree:             &ParseTree{Source: input},
	}

	log.Timed("parsing", input.Name, func() {
		p.parse()
	})

	return p.tree, p.deps
}

func (v *parser) err(err string, stuff ...interface{}) {
	v.errPos(err, stuff...)
}

func (v *parser) errToken(err string, stuff ...interface{}) {
	tok := v.peek(0)
	if tok != nil {
		v.errTokenSpecific(tok, err, stuff...)
	} else {
		lastTok := v.input.Tokens[len(v.input.Tokens)-1]
		v.errTokenSpecific(lastTok, err, stuff...)
	}

}

func (v *parser) errPos(err string, stuff ...interface{}) {
	tok := v.peek(0)
	if tok != nil {
		v.errPosSpecific(v.peek(0).Where.Start(), err, stuff...)
	} else {
		lastTok := v.input.Tokens[len(v.input.Tokens)-1]
		v.errPosSpecific(lastTok.Where.Start(), err, stuff...)
	}

}

func (v *parser) errTokenSpecific(tok *lexer.Token, err string, stuff ...interface{}) {
	v.dumpRules()
	log.Errorln("parser",
		util.TEXT_RED+util.TEXT_BOLD+"error:"+util.TEXT_RESET+" [%s:%d:%d] %s",
		tok.Where.Filename, tok.Where.StartLine, tok.Where.StartChar,
		fmt.Sprintf(err, stuff...))

	log.Error("parser", v.input.MarkSpan(tok.Where))

	os.Exit(util.EXIT_FAILURE_PARSE)
}

func (v *parser) errPosSpecific(pos lexer.Position, err string, stuff ...interface{}) {
	v.dumpRules()
	log.Errorln("parser",
		util.TEXT_RED+util.TEXT_BOLD+"error:"+util.TEXT_RESET+" [%s:%d:%d] %s",
		pos.Filename, pos.Line, pos.Char,
		fmt.Sprintf(err, stuff...))

	log.Error("parser", v.input.MarkPos(pos))

	os.Exit(util.EXIT_FAILURE_PARSE)
}

// rule operations

func (v *parser) pushRule(name string) {
	v.ruleStack = append(v.ruleStack, name)
}

func (v *parser) popRule() {
	v.ruleStack = v.ruleStack[:len(v.ruleStack)-1]
}

func (v *parser) dumpRules() {
	log.Debugln("parser", strings.Join(v.ruleStack, " / "))
}

// peek 向前亏看ahead个Token。
func (v *parser) peek(ahead int) *lexer.Token {
	if ahead < 0 {
		panic(fmt.Sprintf("Tried to peek a negative number: %d", ahead))
	}

	if v.currentToken+ahead >= len(v.input.Tokens) {
		return nil
	}

	return v.input.Tokens[v.currentToken+ahead]
}

// consumeToken 消化一个Token，即分析器向前进一步
func (v *parser) consumeToken() *lexer.Token {
	ret := v.peek(0)
	v.currentToken++
	return ret
}

// consumeTokens 消化num个Token
func (v *parser) consumeTokens(num int) {
	for i := 0; i < num; i++ {
		v.consumeToken()
	}
}

// tokenMatches 判断前方第ahead个Token是否符合条件：类型是t，内容是content
func (v *parser) tokenMatches(ahead int, t lexer.TokenType, contents string) bool {
	tok := v.peek(ahead)
	return tok != nil && tok.Type == t && (contents == "" || (tok.Contents == contents))
}

// tokensMatch 判断接下来多个Token是否符合给出的条件。参数args中，每两个参数是一组条件：分辨是类型和内容
func (v *parser) tokensMatch(args ...interface{}) bool {
	if len(args)%2 != 0 {
		panic("passed uneven args to tokensMatch")
	}

	for i := 0; i < len(args)/2; i++ {
		if !(v.tokenMatches(i, args[i*2].(lexer.TokenType), args[i*2+1].(string))) {
			return false
		}
	}
	return true
}

// getPrecedence 获取二元操作符对应的优先级
func (v *parser) getPrecedence(op BinOpType) int {
	if p := v.binOpPrecedences[op]; p > 0 {
		return p
	}
	return -1
}

// nextIs 判断下一个Token是否是typ类型
func (v *parser) nextIs(typ lexer.TokenType) bool {
	next := v.peek(0)
	if next == nil {
		v.err("Expected token of type %s, got EOF", typ)
	}
	return next.Type == typ
}

// optional 如果下一个Token符合类型typ，内容val，则消化它；如果不符合，则忽略。用于某些可以出现也可以不出现的语法部件，例如函数定义时，可以指定返回类型，也可以不指定。
func (v *parser) optional(typ lexer.TokenType, val string) *lexer.Token {
	if v.tokenMatches(0, typ, val) {
		return v.consumeToken()
	} else {
		return nil
	}
}

// expect 期待下一个Token必须符合类型typ，内容val。如果不符合，则报错退出。用于判断必须出现的语法组合。
// 例如，定义函数时，在解析完函数名称后，下一个Token必须是一个'('。
func (v *parser) expect(typ lexer.TokenType, val string) *lexer.Token {
	if !v.tokenMatches(0, typ, val) {
		tok := v.peek(0)
		if tok == nil {
			if val != "" {
				v.err("Expected `%s` (%s), got EOF", val, typ)
			} else {
				v.err("Expected %s, got EOF", typ)
			}
		} else {
			if val != "" {
				v.errToken("Expected `%s` (%s), got `%s` (%s)", val, typ, tok.Contents, tok.Type)
			} else {
				v.errToken("Expected %s, got %s (`%s`)", typ, tok.Type, tok.Contents)
			}
		}

	}
	return v.consumeToken()
}

// parse 语法分析器的主方法，开启分析的循环
func (v *parser) parse() {
	for v.peek(0) != nil {
		if n := v.parseDecl(true); n != nil { // 各种定义块，如函数定义，常量定义等
			v.tree.AddNode(n)
		} else if n := v.parseToplevelDirective(); n != nil { // 顶层指令，如use语句等
			v.tree.AddNode(n)
		} else {
			v.err("Unexpected token at toplevel: `%s` (%s)", v.peek(0).Contents, v.peek(0).Type)
		}
	}
}

// parseToplevelDirective 分析顶层指令
func (v *parser) parseToplevelDirective() ParseNode {
	defer un(trace(v, "toplevel-directive"))

	// 分析use语句。注：由于现在已把Ark的 #use 改为了直接用use，所以这段逻辑应当独立出去。
	// use 语句现在只支持最简单的 use a.b.c.d 这样的形式
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_USE) {
		directive := v.consumeToken()

		module := v.parseName()
		if module == nil {
			v.errPosSpecific(directive.Where.End(), "Expected name after use directive")
		}

		v.deps = append(v.deps, module)

		res := &UseDirectiveNode{Module: module}
		res.SetWhere(lexer.NewSpan(directive.Where.Start(), module.Where().End()))
		return res
	}

	// 顶层指令应当以 # 开头
	if !v.tokensMatch(lexer.Operator, "#", lexer.Identifier, "") {
		return nil
	}
	start := v.expect(lexer.Operator, "#")

	// 解析指令名称
	directive := v.expect(lexer.Identifier, "")
	switch directive.Contents {
	case "link": // 现在只支持 #link，之前还有 #use，但在喾语言中将它独立出去了。
		library := v.expect(lexer.String, "")
		res := &LinkDirectiveNode{Library: NewLocatedString(library)}
		res.SetWhere(lexer.NewSpanFromTokens(start, library))
		return res

	default:
		v.errTokenSpecific(directive, "No such directive `%s`", directive.Contents)
		return nil
	}
}

// parseDocComments 分析文档注释
func (v *parser) parseDocComments() []*DocComment {
	defer un(trace(v, "doccomments"))

	var dcs []*DocComment

	for v.nextIs(lexer.Doccomment) {
		tok := v.consumeToken()

		var contents string
		if strings.HasPrefix(tok.Contents, "/**") {
			contents = tok.Contents[3 : len(tok.Contents)-2]
		} else if strings.HasPrefix(tok.Contents, "///") {
			contents = tok.Contents[3:]
		} else {
			panic(fmt.Sprintf("How did this doccomment get through the lexer??\n`%s`", tok.Contents))
		}

		dcs = append(dcs, &DocComment{Where: tok.Where, Contents: contents})
	}

	return dcs
}

// parseAttributes 分析标注
// 注意：现在标注只允许顶层定义块使用，支持如下格式：
// 1. [key]， 例如 [c] 表示一个函数是C语言的extern函数
// 2. [key=value]
// 多个标注间用逗号分隔，例如：[a=1,b=2]
func (v *parser) parseAttributes() AttrGroup {
	defer un(trace(v, "attributes"))

	if !v.tokensMatch(lexer.Separator, "[") {
		return nil
	}
	attrs := make(AttrGroup)

	for v.tokensMatch(lexer.Separator, "[") {
		v.consumeToken()
		for {
			attr := &Attr{}

			keyToken := v.expect(lexer.Identifier, "")
			attr.SetPos(keyToken.Where.Start())
			attr.Key = keyToken.Contents

			if v.tokenMatches(0, lexer.Operator, "=") {
				v.consumeToken()
				attr.Value = v.expect(lexer.String, "").Contents
			}

			if attrs.Set(attr.Key, attr) {
				v.err("Duplicate attribute `%s`", attr.Key)
			}

			if !v.tokenMatches(0, lexer.Separator, ",") {
				break
			}
			v.consumeToken()
		}

		v.expect(lexer.Separator, "]")
	}

	return attrs
}

func (v *parser) parseName() *NameNode {
	defer un(trace(v, "name"))

	if !v.nextIs(lexer.Identifier) {
		return nil
	}

	var parts []LocatedString
	for {
		part := v.expect(lexer.Identifier, "")
		parts = append(parts, NewLocatedString(part))

		if !v.tokenMatches(0, lexer.Operator, "::") {
			break
		}
		v.consumeToken()
	}

	name, parts := parts[len(parts)-1], parts[:len(parts)-1]
	res := &NameNode{Modules: parts, Name: name}
	if len(parts) > 0 {
		res.SetWhere(lexer.NewSpan(parts[0].Where.Start(), name.Where.End()))
	} else {
		res.SetWhere(name.Where)
	}
	return res
}

// parseDecl 解析各种定义语句。
// 包括：
//    - 类型定义 TypeDecl
//    - 函数定义 FuncDecl
//    - 变量定义 VarDecl （其实主要是常量）
//    - 解构变量定义 DestructVarDecl 这个是支持多变量定义的特殊语法
func (v *parser) parseDecl(isTopLevel bool) ParseNode {
	defer un(trace(v, "decl"))

	var res ParseNode

	// 先解析可选的文档注释和标注
	docComments := v.parseDocComments()
	attrs := v.parseAttributes()

	// 解析pub属性
	var pub bool
	if isTopLevel {
		if v.tokenMatches(0, lexer.Identifier, KEYWORD_PUB) {
			pub = true
			v.consumeToken()
		}
	}
	// 解析不同类型的定义块
	if typeDecl := v.parseTypeDecl(isTopLevel); typeDecl != nil { // 类型定义，即type语句
		res = typeDecl
	} else if funcDecl := v.parseFuncDecl(isTopLevel); funcDecl != nil { // 函数定义
		res = funcDecl
	} else if varDecl := v.parseVarDecl(isTopLevel); varDecl != nil { // 变量定义
		res = varDecl
	} else if varTupleDecl := v.parseDestructVarDecl(isTopLevel); varTupleDecl != nil { // 多变量定义
		res = varTupleDecl
	} else {
		return nil
	}

	// 将开头解析的pub属性、文档注释和标注添加到解析结果中
	res.(DeclNode).SetPublic(pub)

	if len(docComments) != 0 {
		res.SetDocComments(docComments)
	}

	if attrs != nil {
		res.SetAttrs(attrs)
	}

	return res
}

// parseFuncDecl 解析函数定义
func (v *parser) parseFuncDecl(isTopLevel bool) *FunctionDeclNode {
	fn := v.parseFunc(false, isTopLevel)
	if fn == nil {
		return nil
	}

	res := &FunctionDeclNode{Function: fn}
	res.SetWhere(fn.Where())
	return res
}

// parseLambdaExpr 解析lambda函数定义
func (v *parser) parseLambdaExpr() *LambdaExprNode {
	fn := v.parseFunc(true, false)
	if fn == nil {
		return nil
	}

	res := &LambdaExprNode{Function: fn}
	res.SetWhere(fn.Where())
	return res
}

// parseFunc 分析函数
// If lambda is true, we're parsing an expression.
// If lambda is false, we're parsing a proper function declaration.
func (v *parser) parseFunc(lambda bool, topLevelNode bool) *FunctionNode {
	defer un(trace(v, "func"))

	// 函数头
	funcHeader := v.parseFuncHeader(lambda)
	if funcHeader == nil {
		return nil
	}

	var body *BlockNode
	var stat, expr ParseNode
	var end lexer.Position

	if v.tokenMatches(0, lexer.Separator, ";") { // 直接结束。即函数声明。注：除了定义外部函数的情况，不应该出现没有函数体的函数定义。应该去掉这种情形。
		terminator := v.consumeToken()
		end = terminator.Where.End()
	} else if v.tokenMatches(0, lexer.Operator, "=>") { // lambda表达式
		v.consumeToken()

		isCond := false
		if stat = v.parseStat(); stat != nil {
			end = stat.Where().End()
		} else if stat = v.parseConditionalStat(); stat != nil {
			end = stat.Where().End()
			isCond = true
		} else if expr = v.parseExpr(); expr != nil {
			end = expr.Where().End()
		} else {
			v.err("Expected valid statement or expression after => operator in function declaration")
		}

		if topLevelNode && !isCond {
			v.expect(lexer.Separator, ";")
		}
	} else { // 函数体
		body = v.parseBlock()
		if body == nil {
			v.err("Expected block after function declaration, or terminating semi-colon")
		}
		end = body.Where().End()
	}

	res := &FunctionNode{Header: funcHeader, Body: body, Stat: stat, Expr: expr}
	res.SetWhere(lexer.NewSpan(funcHeader.Where().Start(), end))
	return res
}

// If lambda is true, don't parse name and set Anonymous to true.
func (v *parser) parseFuncHeader(lambda bool) *FunctionHeaderNode {
	defer un(trace(v, "funcheader"))

	// 函数头必须以fun关键字开头。
	// TODO: 未来应当让lambda不需要使用fun，而是直接 (a int, b int) => a + b 即可
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_FUN) {
		return nil
	}
	startToken := v.consumeToken()

	// 如果fun后面有var关键字，表示该方法可以修改对象成员。否则，方法默认是不能够修改对象成员的
	var mutable *lexer.Token
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
		mutable = v.consumeToken()
	}

	// 解析static属性
	var static bool
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_STATIC) {
		static = true
		v.consumeToken()
	}

	// static用于静态内部函数；var用于方法的声明，因此不应该同时出现
	if mutable != nil && static {
		v.errPos("static and var functions should not happend at the same time")
	}

	res := &FunctionHeaderNode{}

	if !lambda {
		// 方法的类名。
		// 格式：fun (a: String) startsWidth(head string) bool
		// TODO: 未来应当改为类似Kotlin的方法定义格式： fun String.startsWith(head: string) bool，不过这样需要增加关键字 this用来指代当前对象
		// parses the function receiver if there is one.
		if v.tokenMatches(0, lexer.Identifier, "") {

			pos := v.currentToken
			tok := v.peek(0)

			if static {
				typ := v.parseNamedType()
				if typ != nil && v.tokenMatches(0, lexer.Separator, ".") {
					res.StaticReceiverType = typ
					v.expect(lexer.Separator, ".")
				} else {
					// 解析类型失败，或者后面没有"."，回退到前面的位置，尝试直接解析函数名称
					v.currentToken = pos
				}
			} else {
				// 先尝试解析一个类型名称，后面应当接着一个"."
				typ := v.parseTypeReference(true, false, true)
				wtyp := typ
				if mutable != nil {
					ptyp := &PointerTypeNode{Mutable: mutable != nil, TargetType: typ}
					wtyp = &TypeReferenceNode{Type: ptyp}
				}

				if typ != nil && v.tokenMatches(0, lexer.Separator, ".") {

					res.Receiver = &VarDeclNode{
						Name: NewLocatedString(&lexer.Token{
							Type:     lexer.Identifier,
							Contents: "this",
							Where:    tok.Where,
						}),
						Type:             wtyp,
						IsImplicit:       true,
						IsMethodReceiver: true,
					}
					if mutable != nil {
						res.Receiver.Mutable = NewLocatedString(mutable)
					}
					v.expect(lexer.Separator, ".")

				} else {
					// 解析类型失败，或者后面没有"."，回退到前面的位置，尝试直接解析函数名称
					v.currentToken = pos
				}
			}

			/*
				// static 方法，只有类名，没有对象名。实例 fun (String) make(len int) string
				if v.tokensMatch(lexer.Identifier, "", lexer.Separator, ")") {
					res.StaticReceiverType = v.parseNamedType()
					if res.StaticReceiverType == nil {
						v.errToken("Expected type name in method receiver, found `%s`", v.peek(0).Contents)
					}
				} else { // 普通方法，有对象名和类名
					res.Receiver = v.parseParaDecl(true)
					if res.Receiver == nil {
						v.errToken("Expected variable declaration in method receiver, found `%s`", v.peek(0).Contents)
					}
				}
			*/
		}

		// 函数名
		name := v.expect(lexer.Identifier, "")
		res.Name = NewLocatedString(name)
	}

	// 函数名后面接着泛型声明
	genericSigil := v.parseGenericSigil()

	// 然后是参数列表，以(开头
	v.expect(lexer.Separator, "(")

	var args []*VarDeclNode
	variadic := false
	// 解析函数参数，以逗号分隔，以)结尾
	for {
		if v.tokenMatches(0, lexer.Separator, ")") {
			break
		}

		// 可变参数，即...
		if v.tokensMatch(lexer.Separator, ".", lexer.Separator, ".", lexer.Separator, ".") {
			v.consumeTokens(3)
			if !variadic {
				variadic = true
			} else {
				v.err("Duplicate `...` in function arguments")
			}
		} else { // 否则每个参数是一个变量定义块
			arg := v.parseParaDecl(false)
			if arg == nil {
				v.err("Expected valid variable declaration in function args")
			}
			args = append(args, arg)
		}

		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}

	// 参数列表结束
	maybeEndToken := v.expect(lexer.Separator, ")")

	// 解析返回类型。可能为空
	var returnType *TypeReferenceNode
	returnType = v.parseTypeReference(true, false, true)

	res.Arguments = args
	res.Variadic = variadic
	res.GenericSigil = genericSigil
	res.Anonymous = lambda

	if returnType != nil {
		res.ReturnType = returnType
		res.SetWhere(lexer.NewSpan(startToken.Where.Start(), returnType.Where().End()))
	} else {
		res.SetWhere(lexer.NewSpanFromTokens(startToken, maybeEndToken))
	}

	return res
}

// parseTypeDecl 分析类型定义
func (v *parser) parseTypeDecl(isTopLevel bool) *TypeDeclNode {
	defer un(trace(v, "typdecl"))

	// 类型定义以 type 开头
	if !v.tokenMatches(0, lexer.Identifier, "type") {
		return nil
	}

	startToken := v.consumeToken()

	// 接着是类型的名称
	name := v.expect(lexer.Identifier, "")
	// 类型名称不能是关键字
	if IsReservedKeyword(name.Contents) {
		v.err("Cannot use reserved keyword `%s` as type name", name.Contents)
	}

	// 如果直接遇到"{"，则认为后面是一个struct结构体声明。
	var typ ParseNode
	if v.tokenMatches(0, lexer.Separator, "{") {
		typ = v.parseStructType(false) // 这里的结构体不需要 struct 关键字
	} else {
		// 解析其他具体类型
		typ = v.parseType(true, false, true)
	}

	// 根据解析结果构造语法节点
	res := &TypeDeclNode{
		Name: NewLocatedString(name),
		Type: typ,
	}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), typ.Where().End()))

	return res
}

func (v *parser) parseGenericSigil() *GenericSigilNode {
	defer un(trace(v, "genericsigil"))

	if !v.tokenMatches(0, lexer.Operator, "<") {
		return nil
	}
	startToken := v.consumeToken()

	var parameters []*TypeParameterNode
	for {
		parameter := v.parseTypeParameter()
		if parameter == nil {
			v.err("Expected valid type parameter in generic sigil")
		}
		parameters = append(parameters, parameter)

		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}
	endToken := v.expect(lexer.Operator, ">")

	res := &GenericSigilNode{GenericParameters: parameters}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

func (v *parser) parseTypeParameter() *TypeParameterNode {
	name := v.expect(lexer.Identifier, "")

	var constraints []*TypeReferenceNode
	if v.tokenMatches(0, lexer.Operator, ":") {
		v.consumeToken()
		for {
			constraint := v.parseTypeReference(true, false, false)
			if constraint == nil {
				v.err("Expected valid name in type restriction")
			}
			constraints = append(constraints, constraint)

			if !v.tokenMatches(0, lexer.Operator, "&") {
				break
			}
			v.consumeToken()
		}
	}

	res := &TypeParameterNode{Name: NewLocatedString(name), Constraints: constraints}
	if idx := len(constraints) - 1; idx >= 0 {
		res.SetWhere(lexer.NewSpan(name.Where.Start(), constraints[idx].Where().End()))
	} else {
		res.SetWhere(lexer.NewSpanFromTokens(name, name))
	}
	return res
}

func (v *parser) parseEnumEntry() *EnumEntryNode {
	defer un(trace(v, "enumentry"))

	if !v.nextIs(lexer.Identifier) {
		return nil
	}
	name := v.consumeToken()

	if IsReservedKeyword(name.Contents) {
		v.err("Cannot use reserved keyword `%s` as name for enum entry", name.Contents)
	}

	var value *NumberLitNode
	var structBody *StructTypeNode
	var tupleBody *TupleTypeNode
	var lastPos lexer.Position
	if v.tokenMatches(0, lexer.Operator, "=") {
		v.consumeToken()

		value = v.parseNumberLit()
		if value == nil || value.IsFloat {
			v.err("Expected valid integer after `=` in enum entry")
		}
		lastPos = value.Where().End()
	} else if tupleBody = v.parseTupleType(true); tupleBody != nil {
		lastPos = tupleBody.Where().End()
	} else if structBody = v.parseStructType(false); structBody != nil {
		lastPos = structBody.Where().End()
	}

	res := &EnumEntryNode{Name: NewLocatedString(name), Value: value, TupleBody: tupleBody, StructBody: structBody}
	if value != nil || structBody != nil || tupleBody != nil {
		res.SetWhere(lexer.NewSpan(name.Where.Start(), lastPos))
	} else {
		res.SetWhere(name.Where)
	}
	return res
}

func (v *parser) parseVarDecl(isTopLevel bool) *VarDeclNode {
	defer un(trace(v, "vardecl"))

	body := v.parseVarDeclBody(false)
	if body == nil {
		return nil
	}

	return body
}

// parseParaDecl 解析变量声明块。用于普通变量的定义，也用于函数定义中的变量列表。
// 实例：a: string
func (v *parser) parseParaDecl(isReceiver bool) *VarDeclNode {
	defer un(trace(v, "vardeclbody"))

	startPos := v.currentToken

	// 可修改变量，默认是不可修改的，因此需要加var来指定可修改
	var mutable *lexer.Token
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
		mutable = v.consumeToken()
	}

	// 接着是变量名称
	if !v.tokenMatches(0, lexer.Identifier, "") {
		v.currentToken = startPos
		return nil
	}
	name := v.consumeToken()

	// 变量类型
	varType := v.parseTypeReference(true, false, true)
	if varType == nil && !v.tokenMatches(0, lexer.Operator, "=") {
		v.err("Expected valid type in variable declaration")
	}

	// 赋值语句。
	var value ParseNode
	if v.tokenMatches(0, lexer.Operator, "=") {
		v.consumeToken()

		// =后面可能是一个结构体常量
		value = v.parseCompositeLiteral()
		if value == nil {
			// 也可能是一个表达式
			value = v.parseExpr()
		}

		if value == nil {
			v.err("Expected valid expression after `=` in variable declaration")
		}
	}

	res := &VarDeclNode{
		Name:       NewLocatedString(name),
		Type:       varType,
		IsImplicit: isReceiver,
	}
	start := name.Where.Start()
	if mutable != nil {
		res.Mutable = NewLocatedString(mutable)
		start = mutable.Where.Start()
	}

	var end lexer.Position
	if value != nil {
		res.Value = value
		end = value.Where().End()
	} else {
		end = varType.Where().End()
	}

	res.SetWhere(lexer.NewSpan(start, end))
	return res
}

// parseVarDeclBody 解析变量声明块。用于普通变量的定义，也用于函数定义中的变量列表。
// 实例：a: string
func (v *parser) parseVarDeclBody(isReceiver bool) *VarDeclNode {
	defer un(trace(v, "vardeclbody"))

	startPos := v.currentToken

	// 可修改变量，默认是不可修改的，因此需要加var来指定可修改
	var mutable *lexer.Token
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_LET) {
		mutable = nil
		v.consumeToken()
	} else if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
		mutable = v.consumeToken()
	} else {
		return nil
	}

	// 变量名接着一个
	if !v.tokenMatches(0, lexer.Identifier, "") {
		v.currentToken = startPos
		return nil
	}

	name := v.consumeToken()

	// 变量类型
	varType := v.parseTypeReference(true, false, true)
	if varType == nil && !v.tokenMatches(0, lexer.Operator, "=") {
		v.err("Expected valid type in variable declaration")
	}

	// 赋值语句。
	var value ParseNode
	if v.tokenMatches(0, lexer.Operator, "=") {
		v.consumeToken()

		// =后面可能是一个结构体常量
		value = v.parseCompositeLiteral()
		if value == nil {
			// 也可能是一个表达式
			value = v.parseExpr()
		}

		if value == nil {
			v.err("Expected valid expression after `=` in variable declaration")
		}
	}

	res := &VarDeclNode{
		Name:       NewLocatedString(name),
		Type:       varType,
		IsImplicit: isReceiver,
	}
	start := name.Where.Start()
	if mutable != nil {
		res.Mutable = NewLocatedString(mutable)
		start = mutable.Where.Start()
	}

	var end lexer.Position
	if value != nil {
		res.Value = value
		end = value.Where().End()
	} else {
		end = varType.Where().End()
	}

	res.SetWhere(lexer.NewSpan(start, end))
	return res
}

// parseDestructVarDecl 解构变量声明语句。即多变量声明。
// 实例：(a, b) := (1, 2)
// 注：这里似乎没有变量的类型声明，难道只支持类型推导？
func (v *parser) parseDestructVarDecl(isTopLevel bool) *DestructVarDeclNode {
	defer un(trace(v, "destructvardecl"))
	startPos := v.currentToken

	// 以(开头
	if !v.tokenMatches(0, lexer.Separator, "(") {
		return nil
	}
	start := v.consumeToken()

	var names []LocatedString
	var mutable []bool
	// 循环解析多个变量声明
	for {
		// 解析var关键字
		isMutable := false
		if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
			isMutable = true
			v.consumeToken()
		}

		// 之后必须是变量名称
		if !v.nextIs(lexer.Identifier) {
			// TODO(#655):
			//v.errPos("Expected identifier in tuple destructuring variable declaration, got %s", v.peek(0).Type)
			v.currentToken = startPos
			return nil
		}
		name := v.consumeToken()

		names = append(names, NewLocatedString(name))
		mutable = append(mutable, isMutable)

		// 逗号分隔
		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}

	// )结尾
	v.expect(lexer.Separator, ")")

	// 接着必须是 := 符号
	if !v.tokenMatches(0, lexer.Operator, ":") {
		v.currentToken = startPos
		return nil
	}
	v.expect(lexer.Operator, ":")
	v.expect(lexer.Operator, "=")

	// 解析变量值，它是一个表达式
	value := v.parseExpr()
	if value == nil {
		v.err("Expected valid expression after tuple destructuring variable declaration")
	}

	res := &DestructVarDeclNode{
		Names:   names,
		Mutable: mutable,
		Value:   value,
	}
	res.SetWhere(lexer.NewSpan(start.Where.Start(), value.Where().End()))
	return res
}

// parseConditionalStat 解析条件语句
func (v *parser) parseConditionalStat() ParseNode {
	defer un(trace(v, "conditionalstat"))

	var res ParseNode

	// 分别尝试不同的条件语句
	if ifStat := v.parseIfStat(); ifStat != nil { // if 语句
		res = ifStat
	} else if matchStat := v.parseMatchStat(); matchStat != nil { // match 语句
		res = matchStat
	} else if loopStat := v.parseLoopStat(); loopStat != nil { // for 循环语句
		res = loopStat
	}

	return res
}

// parseStat 解析普通语句
func (v *parser) parseStat() ParseNode {
	defer un(trace(v, "stat"))

	var res ParseNode

	if breakStat := v.parseBreakStat(); breakStat != nil { // break 语句
		res = breakStat
	} else if continueStat := v.parseContinueStat(); continueStat != nil { // continue 语句
		res = continueStat
	} else if deferStat := v.parseDeferStat(); deferStat != nil { // defer 语句
		res = deferStat
	} else if returnStat := v.parseReturnStat(); returnStat != nil { // return 语句
		res = returnStat
	} else if callStat := v.parseCallStat(); callStat != nil { // 函数调用语句
		res = callStat
	} else if assignStat := v.parseAssignStat(); assignStat != nil { // 赋值语句
		res = assignStat
	} else if binopAssignStat := v.parseBinopAssignStat(); binopAssignStat != nil { // 二元赋值语句
		res = binopAssignStat
	}

	return res
}

// parseDeferStat 解析defer语句
func (v *parser) parseDeferStat() *DeferStatNode {
	defer un(trace(v, "deferstat"))

	// 以关键字defer开头
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_DEFER) {
		return nil
	}
	startToken := v.consumeToken()

	// 后接一个函数调用表达式
	// TODO: 应当支持任意语句，比如一段代码块
	call, ok := v.parseExpr().(*CallExprNode)
	if !ok {
		v.err("Expected valid call expression in defer statement")
	}

	res := &DeferStatNode{Call: call}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), call.Where().End()))
	return res
}

// parseIfStat 解析if条件语句
func (v *parser) parseIfStat() *IfStatNode {
	defer un(trace(v, "ifstat"))

	// 以if关键字开头
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_IF) {
		return nil
	}
	startToken := v.consumeToken()

	var parts []*ConditionBodyNode
	var lastPart *ConditionBodyNode
	for {
		// 条件表达式。注：这里和Go一样，if后面的条件可以不用括号
		condition := v.parseExpr()
		if condition == nil {
			v.err("Expected valid expression as condition in if statement")
		}

		// 条件执行代码块
		body := v.parseBlock()
		if body == nil {
			v.err("Expected valid block after condition in if statement")
		}

		lastPart = &ConditionBodyNode{Condition: condition, Body: body}
		lastPart.SetWhere(lexer.NewSpan(condition.Where().Start(), body.Where().End()))
		parts = append(parts, lastPart)

		// 支持else if多次条件判断
		if !v.tokensMatch(lexer.Identifier, KEYWORD_ELSE, lexer.Identifier, KEYWORD_IF) {
			break
		}
		v.consumeTokens(2)
	}

	// 如果if语句后面接着有else关键字，则继续解析else分支
	var elseBody *BlockNode
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_ELSE) {
		v.consumeToken()

		elseBody = v.parseBlock()
		if elseBody == nil {
			v.err("Expected valid block after `else` keyword in if statement")
		}
	}

	res := &IfStatNode{Parts: parts, ElseBody: elseBody}
	if elseBody != nil {
		res.SetWhere(lexer.NewSpan(startToken.Where.Start(), elseBody.Where().End()))
	} else {
		res.SetWhere(lexer.NewSpan(startToken.Where.Start(), lastPart.Where().End()))
	}
	return res
}

// parseMatchStat 解析模式匹配语句
func (v *parser) parseMatchStat() *MatchStatNode {
	defer un(trace(v, "matchstat"))

	// 以match关键字开头
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_MATCH) {
		return nil
	}
	startToken := v.consumeToken()

	// 接着是要判断匹配的表达式
	value := v.parseExpr()
	if value == nil {
		v.err("Expected valid expresson as value in match statement")
	}

	// 然后是匹配代码块，以{}包含
	v.expect(lexer.Separator, "{")

	var cases []*MatchCaseNode
	// 循环解析多个匹配项
	for {
		// 以}结尾
		if v.tokenMatches(0, lexer.Separator, "}") {
			break
		}

		// 解析一个匹配模式
		pattern := v.parseMatchPattern()
		if pattern == nil {
			v.err("Expected valid pattern in match statement")
		}

		// 匹配模式与操作间用=>分隔
		v.expect(lexer.Operator, "=>")

		// 操作代码
		var body ParseNode
		if v.tokenMatches(0, lexer.Separator, "{") { // 可以是代码块
			body = v.parseBlock()
		} else { // 也可以是单个语句
			body = v.parseStat()
		}
		if body == nil {
			v.err("Expected valid arm statement in match clause")
		}

		// 各个模式项之间以逗号分隔
		v.expect(lexer.Separator, ",")

		caseNode := &MatchCaseNode{Pattern: pattern, Body: body}
		caseNode.SetWhere(lexer.NewSpan(pattern.Where().Start(), body.Where().End()))
		cases = append(cases, caseNode)
	}

	endToken := v.expect(lexer.Separator, "}")

	res := &MatchStatNode{Value: value, Cases: cases}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// parseMatchPattern 解析匹配模式
func (v *parser) parseMatchPattern() ParseNode {
	defer un(trace(v, "matchpattern"))
	if numLit := v.parseNumberLit(); numLit != nil { // 数字
		return numLit
	} else if stringLit := v.parseStringLit(); stringLit != nil { // 字符串
		return stringLit
	} else if discardAccess := v.parseDiscardAccess(); discardAccess != nil { // 通配符 _
		return discardAccess
	} else if enumPattern := v.parseEnumPattern(); enumPattern != nil { // 枚举值
		return enumPattern
	}
	return nil
}

// parseDiscardAccess 解析匹配通配符 _
func (v *parser) parseDiscardAccess() *DiscardAccessNode {
	defer un(trace(v, "discardaccess"))
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_DISCARD) {
		return nil
	}
	token := v.expect(lexer.Identifier, KEYWORD_DISCARD)

	res := &DiscardAccessNode{}
	res.SetWhere(token.Where)
	return res
}

// parseEnumPattern 解析枚举模式
func (v *parser) parseEnumPattern() *EnumPatternNode {
	defer un(trace(v, "enumpattern"))
	enumName := v.parseName()
	if enumName == nil {
		return nil
	}

	res := &EnumPatternNode{
		MemberName: enumName,
	}

	var endParens *lexer.Token
	if v.tokenMatches(0, lexer.Separator, "(") {
		v.consumeToken()

		for {
			if v.tokenMatches(0, lexer.Separator, ")") {
				break
			}

			if !v.nextIs(lexer.Identifier) {
				v.err("Expected identifier in enum pattern")
			}

			name := v.consumeToken()
			res.Names = append(res.Names, NewLocatedString(name))

			if !v.tokenMatches(0, lexer.Separator, ",") {
				break
			}
			v.consumeToken()
		}
		endParens = v.expect(lexer.Separator, ")")
	}

	if endParens != nil {
		res.SetWhere(lexer.NewSpan(enumName.Where().Start(), endParens.Where.End()))
	} else {
		res.SetWhere(enumName.Where())
	}
	return res
}

// parseLoopStat 解析循环语句
func (v *parser) parseLoopStat() *LoopStatNode {
	defer un(trace(v, "loopstat"))

	// 关键字for
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_FOR) {
		return nil
	}
	startToken := v.consumeToken()

	// 条件表达式，可以为空。为空时，即为无限循环。
	condition := v.parseExpr()

	// 循环体
	body := v.parseBlock()
	if body == nil {
		v.err("Expected valid block as body of loop statement ", v.peek(0))
	}

	res := &LoopStatNode{Condition: condition, Body: body}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), body.Where().End()))
	return res
}

// parseReturnStat 解析return语句
func (v *parser) parseReturnStat() *ReturnStatNode {
	defer un(trace(v, "returnstat"))

	// 以关键字return开头
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_RETURN) {
		return nil
	}
	startToken := v.consumeToken()

	// 后接一个值。可以是结构体常量，也可以是一个表达式
	value := v.parseCompositeLiteral()
	if value == nil {
		value = v.parseExpr()
	}

	var end lexer.Position
	if value != nil {
		end = value.Where().End()
	} else {
		end = startToken.Where.End()
	}

	res := &ReturnStatNode{Value: value}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), end))
	return res
}

// parseBreakStat 解析break语句
// 注：这里只支持单独的break，还不支持跳出到指定点
func (v *parser) parseBreakStat() *BreakStatNode {
	defer un(trace(v, "breakstat"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_BREAK) {
		return nil
	}
	startToken := v.consumeToken()

	res := &BreakStatNode{}
	res.SetWhere(startToken.Where)
	return res
}

// parseContinueStat 解析continue语句
func (v *parser) parseContinueStat() *ContinueStatNode {
	defer un(trace(v, "continuestat"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_CONTINUE) {
		return nil
	}
	startToken := v.consumeToken()

	res := &ContinueStatNode{}
	res.SetWhere(startToken.Where)
	return res
}

// parseBlockStat 解析代码块语句
func (v *parser) parseBlockStat() *BlockStatNode {
	defer un(trace(v, "blockstat"))

	// 代码块语句可以以do关键字开头，也可以直接进入{}
	startPos := v.currentToken
	var doToken *lexer.Token
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_DO) {
		doToken = v.consumeToken()
	}

	// 解析代码块，即 {...} 的内容
	body := v.parseBlock()
	if body == nil {
		v.currentToken = startPos
		return nil
	}

	res := &BlockStatNode{Body: body}
	if doToken != nil {
		body.NonScoping = true
		res.SetWhere(lexer.NewSpan(doToken.Where.Start(), body.Where().End()))
	} else {
		res.SetWhere(body.Where())
	}
	return res
}

// parseNode 解析函数体内的用法节点，可以是：声明语句；条件语句；代码块语句；以及普通的赋值或调用语句。
func (v *parser) parseNode() (ParseNode, bool) {
	defer un(trace(v, "node"))

	var ret ParseNode

	is_cond := false

	if decl := v.parseDecl(false); decl != nil { // 可以在函数体内进行各种声明，包括变量、函数等。
		ret = decl
	} else if cond := v.parseConditionalStat(); cond != nil { // 条件流程控制语句
		ret = cond
		is_cond = true
	} else if blockStat := v.parseBlockStat(); blockStat != nil { // 函数体内可以再嵌套代码块
		ret = blockStat
		is_cond = true
	} else if stat := v.parseStat(); stat != nil { // 普通语句
		ret = stat
	}

	return ret, is_cond
}

// parseBlock 解析函数体，必须用{}包括
func (v *parser) parseBlock() *BlockNode {
	defer un(trace(v, "block"))

	// 以{开头
	if !v.tokenMatches(0, lexer.Separator, "{") {
		return nil
	}
	startToken := v.consumeToken()

	// 解析函数体重的各个语法节点，以;分隔
	var nodes []ParseNode
	for {
		node, is_cond := v.parseNode()
		if node == nil {
			break
		}
		if !is_cond {
			v.optional(lexer.Separator, ";")
		}
		nodes = append(nodes, node)
	}

	// 函数体以}结尾
	endToken := v.expect(lexer.Separator, "}")

	res := &BlockNode{Nodes: nodes}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// parseCallStat 解析调用语句
func (v *parser) parseCallStat() *CallStatNode {
	defer un(trace(v, "callstat"))

	startPos := v.currentToken

	// 函数调用语句是一个表达式，并且能够解析成CallExprNode
	// TODO: 为什么不直接写一个parseCallExpr函数呢？
	callExpr, ok := v.parseExpr().(*CallExprNode)
	if !ok {
		v.currentToken = startPos
		return nil
	}

	res := &CallStatNode{Call: callExpr}
	res.SetWhere(lexer.NewSpan(callExpr.Where().Start(), callExpr.Where().End()))
	return res
}

// parseAssignStat 解析赋值语句
func (v *parser) parseAssignStat() ParseNode {
	defer un(trace(v, "assignstat"))

	startPos := v.currentToken

	// 左侧是一个表达式，后接一个=
	accessExpr := v.parseExpr()
	if accessExpr == nil || !v.tokenMatches(0, lexer.Operator, "=") {
		v.currentToken = startPos
		return nil
	}

	// consume '='
	v.consumeToken()

	// 右侧是一个表达式或者结构体常量
	var value ParseNode
	value = v.parseCompositeLiteral()
	if value == nil {
		value = v.parseExpr()
	}

	// not a composite or expr = error
	if value == nil {
		v.err("Expected valid expression in assignment statement")
	}

	res := &AssignStatNode{Target: accessExpr, Value: value}
	res.SetWhere(lexer.NewSpan(accessExpr.Where().Start(), value.Where().End()))
	return res
}

func (v *parser) peekBinop() (BinOpType, int) {
	var str string
	var numTokens int
	if v.tokensMatch(lexer.Operator, ">", lexer.Operator, ">") {
		str = ">>"
		numTokens = 2
	} else {
		str = v.peek(0).Contents
		numTokens = 1
	}

	typ := stringToBinOpType(str)

	return typ, numTokens
}

// parseBinopAssignStat 解析二元赋值语句
// 实例： a += 1
func (v *parser) parseBinopAssignStat() ParseNode {
	defer un(trace(v, "binopassignstat"))

	startPos := v.currentToken

	// 以+=, *=, -=, /= 之类的二元操作符号开头
	accessExpr := v.parseExpr()
	if accessExpr == nil || !v.tokensMatch(lexer.Operator, "", lexer.Operator, "=") {
		v.currentToken = startPos
		return nil
	}

	// 注意，>>=有三个字符。因此要通过 peekBinop单独判断
	typ, numTokens := v.peekBinop()
	if typ == BINOP_ERR || typ.Category() == OP_COMPARISON {
		v.err("Invalid binary operator `%s`", v.peek(0).Contents)
	}
	v.consumeTokens(numTokens)

	// 消化 '='
	v.consumeToken()

	// =右侧可以是表达式或结构体常量
	var value ParseNode
	value = v.parseCompositeLiteral()
	if value == nil {
		value = v.parseExpr()
	}

	// no composite and no expr = err
	if value == nil {
		v.err("Expected valid expression in assignment statement")
	}

	res := &BinopAssignStatNode{Target: accessExpr, Operator: typ, Value: value}
	res.SetWhere(lexer.NewSpan(accessExpr.Where().Start(), value.Where().End()))
	return res
}

// parseTypeReference 分析类型引用
// 注：类型引用包含一个类型，以及泛型列表
// 实例：HashMap<int, string>
func (v *parser) parseTypeReference(doNamed bool, onlyComposites bool, mustParse bool) *TypeReferenceNode {
	// 分析类型，注意这里的类型可以是任何类型，包括函数、数组等，参见parseType的定义。这样可以实现复杂的多层类型嵌套
	typ := v.parseType(doNamed, onlyComposites, mustParse)
	if typ == nil {
		return nil
	}

	var gargs []*TypeReferenceNode
	if v.tokenMatches(0, lexer.Operator, "<") {
		v.consumeToken()

		for {
			// 泛型列表中的每一项也是一个类型引用，因此可以支持泛型嵌套
			typ := v.parseTypeReference(true, false, true)
			if typ == nil {
				v.err("Expected valid type as type parameter")
			}
			gargs = append(gargs, typ)

			if !v.tokenMatches(0, lexer.Separator, ",") {
				break
			}
			v.consumeToken()
		}

		v.expect(lexer.Operator, ">")
	}

	res := &TypeReferenceNode{
		Type:             typ,
		GenericArguments: gargs,
	}

	res.SetWhere(lexer.NewSpan(typ.Where().Start(), typ.Where().End()))
	if len(gargs) > 0 {
		last := gargs[len(gargs)-1]
		res.SetWhere(lexer.NewSpan(typ.Where().Start(), last.Where().End()))
	}

	return res
}

// parseType 分析类型
// NOTE onlyComposites does not affect doRefs.
func (v *parser) parseType(doNamed bool, onlyComposites bool, mustParse bool) ParseNode {
	defer un(trace(v, "type"))

	var res ParseNode
	var attrs AttrGroup

	defer func() {
		if res != nil && attrs != nil {
			res.SetAttrs(attrs)
			// TODO: Update start position of result
		}
	}()

	// attrs = v.parseAttributes()

	if !onlyComposites {
		if v.tokenMatches(0, lexer.Identifier, KEYWORD_FUN) { // 函数类型
			res = v.parseFunctionType()
		} else if v.tokenMatches(0, lexer.Operator, "^") { // 指针类型
			res = v.parsePointerType()
		} else if v.tokenMatches(0, lexer.Operator, "&") { // 引用类型
			res = v.parseReferenceType()
		} else if v.tokenMatches(0, lexer.Separator, "(") { // 元组类型
			res = v.parseTupleType(mustParse)
		} else if v.tokenMatches(0, lexer.Identifier, KEYWORD_INTERFACE) { // 接口类型，这里类似Go的方式，用接口类型指代任何符合接口的类
			res = v.parseInterfaceType()
		}
	}

	if res != nil {
		return res
	}

	if v.tokenMatches(0, lexer.Separator, "[") { // 数组
		res = v.parseArrayType()
	} else if v.tokenMatches(0, lexer.Identifier, KEYWORD_STRUCT) { // 结构体。注：如果要简化自定义结构体类型的定义，就要修改这里。
		res = v.parseStructType(true)
	} else if v.tokenMatches(0, lexer.Identifier, KEYWORD_ENUM) { // 枚举类型
		res = v.parseEnumType()
	} else if doNamed && v.nextIs(lexer.Identifier) { // 普通类型名称。这个功能实际上就是类型别名：如 type MyInt int，实际上相当于D语言的 alias MyInt = int;
		res = v.parseNamedType()
	}

	return res
}

func (v *parser) parseEnumType() *EnumTypeNode {
	defer un(trace(v, "enumtype"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_ENUM) {
		return nil
	}
	startToken := v.consumeToken()

	genericsigil := v.parseGenericSigil()

	v.expect(lexer.Separator, "{")

	var members []*EnumEntryNode
	for {
		if v.tokenMatches(0, lexer.Separator, "}") {
			break
		}

		member := v.parseEnumEntry()
		if member == nil {
			v.err("Expected valid enum entry in enum")
		}
		members = append(members, member)

		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}

	endToken := v.expect(lexer.Separator, "}")

	res := &EnumTypeNode{
		Members:      members,
		GenericSigil: genericsigil,
	}

	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

func (v *parser) parseInterfaceType() *InterfaceTypeNode {
	defer un(trace(v, "interfacetype"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_INTERFACE) {
		return nil
	}

	startToken := v.consumeToken()

	sigil := v.parseGenericSigil()

	v.expect(lexer.Separator, "{")

	// when we hit a };
	// this means our interface is done...
	var functions []*FunctionHeaderNode
	for {
		if v.tokenMatches(0, lexer.Separator, "}") {
			break
		}

		function := v.parseFuncHeader(false)
		if function != nil {
			// TODO trailing comma
			v.expect(lexer.Separator, ",")
			functions = append(functions, function)
		} else {
			v.err("Failed to parse function in interface")
		}
	}

	endToken := v.expect(lexer.Separator, "}")

	res := &InterfaceTypeNode{
		Functions:    functions,
		GenericSigil: sigil,
	}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// parseStructType 解析结构体类型
// 参数requireKeyword表示结构体前面必须有 struct 关键字
func (v *parser) parseStructType(requireKeyword bool) *StructTypeNode {
	defer un(trace(v, "structtype"))

	var startToken *lexer.Token

	var sigil *GenericSigilNode

	if requireKeyword {
		// struct 关键字
		if !v.tokenMatches(0, lexer.Identifier, KEYWORD_STRUCT) {
			return nil
		}
		startToken = v.consumeToken()

		// struct可以是泛型的，这里解析泛型
		sigil = v.parseGenericSigil()

		// “{}"之间是结构体的成员
		v.expect(lexer.Separator, "{")
	} else {
		// 如果不要求关键字，则结构体直接以 "{" 开始
		if !v.tokenMatches(0, lexer.Separator, "{") {
			return nil
		}
		startToken = v.consumeToken()
	}

	var members []*StructMemberNode
	// 循环解析结构体成员，直到遇到“}"
	for {
		// 遇到"}"结束
		if v.tokenMatches(0, lexer.Separator, "}") {
			break
		}

		// 解析一个结构体成员
		member := v.parseStructMember()
		if member == nil {
			v.err("Expected valid member declaration in struct")
		}
		members = append(members, member)

		// 结构体成员间以","分隔
		// TODO：与Go语言类似，去掉","的限制
		if v.tokenMatches(0, lexer.Separator, ",") {
			v.consumeToken()
		}
	}

	endToken := v.expect(lexer.Separator, "}")

	res := &StructTypeNode{Members: members, GenericSigil: sigil}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// parseStructMember 解析一个结构体成员
// 实例： a : int
// TODO 去掉 ":"
func (v *parser) parseStructMember() *StructMemberNode {
	docs := v.parseDocComments()

	// 必须是 "name" 或 "pub name" 开头
	if !(v.tokenMatches(0, lexer.Identifier, "") ||
		v.tokensMatch(lexer.Identifier, KEYWORD_PUB, lexer.Identifier, "")) {
		return nil
	}

	var firstToken *lexer.Token

	// 解析pub关键字
	var isPublic bool
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_PUB) {
		firstToken = v.consumeToken()
		isPublic = true
	}

	// 解析成员名称
	name := v.consumeToken()
	if !isPublic {
		firstToken = name
	}

	// 解析成员类型
	memType := v.parseTypeReference(true, false, true)
	if memType == nil {
		v.err("Expected valid type in struct member")
	}

	res := &StructMemberNode{Name: NewLocatedString(name), Type: memType, Public: isPublic}
	res.SetDocComments(docs)
	res.SetWhere(lexer.NewSpan(firstToken.Where.Start(), memType.Where().End()))
	return res
}

// parseFunctionType 分析函数类型
// 格式实例：fun(int, int) int
func (v *parser) parseFunctionType() *FunctionTypeNode {
	defer un(trace(v, "functiontype"))

	// 函数类型以关键字fun开头
	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_FUN) {
		return nil
	}
	startToken := v.consumeToken()

	// 接着是()包含的参数列表
	if !v.tokenMatches(0, lexer.Separator, "(") {
		v.err("Expected `(` after `func` keyword")
	}
	lastParens := v.consumeToken()

	var pars []*TypeReferenceNode
	variadic := false

	// 解析参数列表
	for {
		// 遇到")"时结束参数列表
		if v.tokenMatches(0, lexer.Separator, ")") {
			lastParens = v.consumeToken()
			break
		}

		// 注意，这里的variadic是C风格的多参数，即  fun(a int, ...) 这样的。这种风格只用于与C语言的互操作。
		// TODO: 未来需要支持类似Go/D风格的真正可变参数，即 fun(a int, b int...)
		if variadic {
			v.err("Variadic signifier must be the last argument in a variadic function")
		}

		// 连续三个...，表示可变参数
		if v.tokensMatch(lexer.Separator, ".", lexer.Separator, ".", lexer.Separator, ".") {
			v.consumeTokens(3)
			if !variadic {
				variadic = true
			} else {
				v.err("Duplicate variadic signifier `...` in function header")
			}
		} else {
			// 解析一个参数的类型
			par := v.parseTypeReference(true, false, true)
			if par == nil {
				v.err("Expected type in function argument, found `%s`", v.peek(0).Contents)
			}

			pars = append(pars, par)
		}

		if v.tokenMatches(0, lexer.Separator, ",") { // 如果遇到逗号，说明后面还有参数，继续循环解析
			v.consumeToken()
			continue
		} else if v.tokenMatches(0, lexer.Separator, ")") { // 遇到)，参数列表结束
			lastParens = v.consumeToken()
			break
		} else {
			v.err("Unexpected `%s`", v.peek(0).Contents)
		}
	}

	// 解析返回类型
	var returnType *TypeReferenceNode
	returnType = v.parseTypeReference(true, false, true)

	var end lexer.Position
	if returnType != nil {
		end = returnType.Where().End()
	} else {
		end = lastParens.Where.End()
	}

	res := &FunctionTypeNode{
		ParameterTypes: pars,
		ReturnType:     returnType,
	}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), end))

	return res
}

// parsePointerType 分析指针类型
// TODO: 尝试将指针类型的操作符由^改为与C/C++/D/Go一致的*。我猜测Ark使用^是为了更简便地避免*在作为指针时与作为乘号时的歧义。
func (v *parser) parsePointerType() *PointerTypeNode {
	defer un(trace(v, "pointertype"))

	mutable, target, where := v.parsePointerlikeType("^")
	if target == nil {
		return nil
	}

	res := &PointerTypeNode{Mutable: mutable, TargetType: target}
	res.SetWhere(where)
	return res
}

// parseReferenceType 分析引用类型
func (v *parser) parseReferenceType() *ReferenceTypeNode {
	defer un(trace(v, "referencetype"))

	mutable, target, where := v.parsePointerlikeType("&")
	if target == nil {
		return nil
	}

	res := &ReferenceTypeNode{Mutable: mutable, TargetType: target}
	res.SetWhere(where)
	return res
}

// parsePointerlikeType 由于指针类型和引用类型的语法分析过程一致，所以把其实现合并到这个函数里了。
func (v *parser) parsePointerlikeType(symbol string) (mutable bool, target *TypeReferenceNode, where lexer.Span) {
	defer un(trace(v, "pointerliketype"))

	// 首先匹配操作符（^或&）
	if !v.tokenMatches(0, lexer.Operator, symbol) {
		return false, nil, lexer.Span{}
	}
	startToken := v.consumeToken()

	// 接着匹配var关键字
	mutable = false
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
		v.consumeToken()
		mutable = true
	}

	// 接着分析类型引用
	target = v.parseTypeReference(true, false, true)
	if target == nil {
		v.err("Expected valid type after '%s' in pointer/reference type", symbol)
	}

	where = lexer.NewSpan(startToken.Where.Start(), target.Where().End())
	return
}

// parseTupleType 分析元组类型。元组类型是由()包含的多个项，每一项以逗号分隔，可以是不同的类型。
// 实例：(int, string, Map<int, string>)
func (v *parser) parseTupleType(mustParse bool) *TupleTypeNode {
	defer un(trace(v, "tupletype"))

	// 首先匹配"("
	if !v.tokenMatches(0, lexer.Separator, "(") {
		return nil
	}
	startToken := v.consumeToken()

	// 接着匹配多个类型引用
	var members []*TypeReferenceNode
	for {
		memberType := v.parseTypeReference(true, false, mustParse)
		if memberType == nil {
			if mustParse {
				v.err("Expected valid type in tuple type")
			} else {
				return nil
			}

		}
		members = append(members, memberType)

		// 类型之间以","分隔
		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}

	// 最后必须匹配到结束符号")"
	endToken := v.expect(lexer.Separator, ")")

	res := &TupleTypeNode{MemberTypes: members}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// parseArrayType 解析数组类型
func (v *parser) parseArrayType() *ArrayTypeNode {
	defer un(trace(v, "arraytype"))

	// 数组以"["开头
	if !v.tokenMatches(0, lexer.Separator, "[") {
		return nil
	}
	startToken := v.consumeToken()

	// 数组长度：数字
	length := v.parseNumberLit()
	if length != nil && length.IsFloat {
		v.err("Expected integer length for array type")
	}

	// 数组以”]”结束
	v.expect(lexer.Separator, "]")

	// 数组元素类型
	memberType := v.parseTypeReference(true, false, true)
	if memberType == nil {
		v.err("Expected valid type in array type")
	}

	res := &ArrayTypeNode{MemberType: memberType}
	if length != nil {
		// TODO: Defend against overflow
		res.Length = int(length.IntValue.Int64())
		res.IsFixedLength = true
	}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), memberType.Where().End()))
	return res
}

// parseNamedType 解析简单类型名称
func (v *parser) parseNamedType() *NamedTypeNode {
	defer un(trace(v, "typereference"))

	name := v.parseName()
	if name == nil {
		return nil
	}

	res := &NamedTypeNode{Name: name}
	res.SetWhere(name.Where())
	return res
}

// parseExpr 解析表达式
func (v *parser) parseExpr() ParseNode {
	defer un(trace(v, "expr"))

	// 先尝试后缀表达式
	pri := v.parsePostfixExpr()
	if pri == nil {
		return nil
	}

	// 再尝试二元操作符表达式
	if bin := v.parseBinaryOperator(0, pri); bin != nil {
		return bin
	}

	return pri
}

func (v *parser) parseBinaryOperator(upperPrecedence int, lhand ParseNode) ParseNode {
	defer un(trace(v, "binop"))

	// TODO: I have a suspicion this might break with some combinations of operators
	startPos := v.currentToken

	tok := v.peek(0)
	if tok.Type != lexer.Operator || v.peek(1).Contents == ";" {
		return nil
	}

	for {
		typ, numTokens := v.peekBinop()

		tokPrecedence := v.getPrecedence(typ)
		if tokPrecedence < upperPrecedence {
			return lhand
		}

		if typ == BINOP_ERR {
			v.err("Invalid binary operator `%s`", v.peek(0).Contents)
		}

		v.consumeTokens(numTokens)

		rhand := v.parsePostfixExpr()
		if rhand == nil {
			v.currentToken = startPos
			return nil
		}

		nextPrecedence := v.getPrecedence(stringToBinOpType(v.peek(0).Contents))
		if tokPrecedence < nextPrecedence {
			rhand = v.parseBinaryOperator(tokPrecedence+1, rhand)
			if rhand == nil {
				v.currentToken = startPos
				return nil
			}
		}

		temp := &BinaryExprNode{
			Lhand:    lhand,
			Rhand:    rhand,
			Operator: typ,
		}
		temp.SetWhere(lexer.NewSpan(lhand.Where().Start(), rhand.Where().Start()))
		lhand = temp
	}
}

// parsePostfixExpr 解析后缀表达式
func (v *parser) parsePostfixExpr() ParseNode {
	defer un(trace(v, "postfixexpr"))

	// 先解析普通表达式
	expr := v.parsePrimaryExpr()
	if expr == nil {
		return nil
	}

	for {
		if v.tokenMatches(0, lexer.Separator, ".") {
			// struct access
			v.consumeToken()
			defer un(trace(v, "structaccess"))

			member := v.expect(lexer.Identifier, "")

			res := &StructAccessNode{Struct: expr, Member: NewLocatedString(member)}
			res.SetWhere(lexer.NewSpan(expr.Where().Start(), member.Where.End()))
			expr = res
		} else if v.tokenMatches(0, lexer.Separator, "[") {
			// array index
			v.consumeToken()
			defer un(trace(v, "arrayindex"))

			index := v.parseExpr()
			if index == nil {
				v.err("Expected valid expression as array index")
			}

			endToken := v.expect(lexer.Separator, "]")

			res := &ArrayAccessNode{Array: expr, Index: index}
			res.SetWhere(lexer.NewSpan(expr.Where().Start(), endToken.Where.End()))
			expr = res
		} else if v.tokenMatches(0, lexer.Separator, "(") {
			// call expr
			v.consumeToken()
			defer un(trace(v, "callexpr"))

			var args []ParseNode
			for {
				if v.tokenMatches(0, lexer.Separator, ")") {
					break
				}

				arg := v.parseCompositeLiteral()
				if arg == nil {
					arg = v.parseExpr()
				}
				if arg == nil {
					v.err("Expected valid expression as call argument")
				}
				args = append(args, arg)

				if !v.tokenMatches(0, lexer.Separator, ",") {
					break
				}
				v.consumeToken()
			}

			endToken := v.expect(lexer.Separator, ")")

			res := &CallExprNode{Function: expr, Arguments: args}
			res.SetWhere(lexer.NewSpan(expr.Where().Start(), endToken.Where.End()))
			expr = res
		} else {
			break
		}
	}

	return expr
}

func (v *parser) parsePrimaryExpr() ParseNode {
	defer un(trace(v, "primaryexpr"))

	var res ParseNode

	if sizeofExpr := v.parseSizeofExpr(); sizeofExpr != nil { // sizeof 表达式
		res = sizeofExpr
	} else if arrayLenExpr := v.parseArrayLenExpr(); arrayLenExpr != nil { // 数组长度表达式
		res = arrayLenExpr
	} else if addrofExpr := v.parseAddrofExpr(); addrofExpr != nil { // 获取地址表达式
		res = addrofExpr
	} else if litExpr := v.parseLitExpr(); litExpr != nil { // 常量表达式
		res = litExpr
	} else if lambdaExpr := v.parseLambdaExpr(); lambdaExpr != nil { // lambda表达式
		res = lambdaExpr
	} else if unaryExpr := v.parseUnaryExpr(); unaryExpr != nil { // 一元操作表达式
		res = unaryExpr
	} else if castExpr := v.parseCastExpr(); castExpr != nil { // 类型转化表达式
		res = castExpr
	} else if name := v.parseName(); name != nil { // 泛型表达式？？
		startPos := v.currentToken

		// Handle discard access
		if len(name.Modules) == 0 && name.Name.Value == "_" {
			res = &DiscardAccessNode{}
			res.SetWhere(name.Where())
		} else {
			var parameters []*TypeReferenceNode
			if v.tokenMatches(0, lexer.Operator, "<") {
				v.consumeToken()

				for {
					typ := v.parseTypeReference(true, false, false)
					if typ == nil {
						break
					}
					parameters = append(parameters, typ)

					if !v.tokenMatches(0, lexer.Separator, ",") {
						break
					}
					v.consumeToken()
				}

				if !v.tokenMatches(0, lexer.Operator, ">") {
					v.currentToken = startPos
					parameters = nil
				} else {
					endToken := v.consumeToken()
					_ = endToken // TODO: Do somethign with end token?
				}
			}

			res = &VariableAccessNode{Name: name, GenericParameters: parameters}
			res.SetWhere(lexer.NewSpan(name.Where().Start(), name.Where().End()))
		}
	}

	return res
}

// len(arr)
func (v *parser) parseArrayLenExpr() *ArrayLenExprNode {
	defer un(trace(v, "arraylenexpr"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_LEN) {
		return nil
	}
	startToken := v.consumeToken()

	v.expect(lexer.Separator, "(")

	var array ParseNode
	array = v.parseCompositeLiteral()
	if array == nil {
		array = v.parseExpr()
	}
	if array == nil {
		v.err("Expected valid expression in array length expression")
	}

	v.expect(lexer.Separator, ")")

	end := v.peek(0)
	res := &ArrayLenExprNode{ArrayExpr: array}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), end.Where.Start()))
	return res
}

// sizeof(expr) 或 sizeof(type)
func (v *parser) parseSizeofExpr() *SizeofExprNode {
	defer un(trace(v, "sizeofexpr"))

	if !v.tokenMatches(0, lexer.Identifier, KEYWORD_SIZEOF) {
		return nil
	}
	startToken := v.consumeToken()

	v.expect(lexer.Separator, "(")

	var typ *TypeReferenceNode
	value := v.parseExpr()
	if value == nil {
		typ = v.parseTypeReference(true, false, true)
		if typ == nil {
			v.err("Expected valid expression or type in sizeof expression")
		}
	}

	endToken := v.expect(lexer.Separator, ")")

	res := &SizeofExprNode{Value: value, Type: typ}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// &expr 或 &var expr
func (v *parser) parseAddrofExpr() *AddrofExprNode {
	defer un(trace(v, "addrofexpr"))
	startPos := v.currentToken

	isReference := false
	if v.tokenMatches(0, lexer.Operator, "&") {
		isReference = true
	} else if v.tokenMatches(0, lexer.Operator, "^") {
		isReference = false
	} else {
		return nil
	}
	startToken := v.consumeToken()

	mutable := false
	if v.tokenMatches(0, lexer.Identifier, KEYWORD_VAR) {
		v.consumeToken()
		mutable = true
	}

	value := v.parseExpr()
	if value == nil {
		// TODO: Restore this error once we go through with #655
		//v.err("Expected valid expression after addrof expression")
		v.currentToken = startPos
		return nil
	}

	res := &AddrofExprNode{Mutable: mutable, Value: value, IsReference: isReference}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), value.Where().End()))
	return res
}

// parseLitExpr 解析各种常量
func (v *parser) parseLitExpr() ParseNode {
	defer un(trace(v, "litexpr"))

	var res ParseNode

	if tupleLit := v.parseTupleLit(); tupleLit != nil { // 常量元组
		res = tupleLit
	} else if boolLit := v.parseBoolLit(); boolLit != nil { // 布尔值 true/false
		res = boolLit
	} else if numberLit := v.parseNumberLit(); numberLit != nil { // 数字常量
		res = numberLit
	} else if stringLit := v.parseStringLit(); stringLit != nil { // 字符串常量
		res = stringLit
	} else if runeLit := v.parseRuneLit(); runeLit != nil { // 字符常量
		res = runeLit
	}

	return res
}

// type(expr)
func (v *parser) parseCastExpr() *CastExprNode {
	defer un(trace(v, "castexpr"))

	startPos := v.currentToken

	typ := v.parseTypeReference(false, false, false)
	if typ == nil || !v.tokenMatches(0, lexer.Separator, "(") {
		v.currentToken = startPos
		return nil
	}
	v.consumeToken()

	value := v.parseExpr()
	if value == nil {
		v.err("Expected valid expression in cast expression")
	}

	endToken := v.expect(lexer.Separator, ")")

	res := &CastExprNode{Type: typ, Value: value}
	res.SetWhere(lexer.NewSpan(typ.Where().Start(), endToken.Where.End()))
	return res
}

// -var | @var | !var | ~var
func (v *parser) parseUnaryExpr() *UnaryExprNode {
	defer un(trace(v, "unaryexpr"))

	startPos := v.currentToken

	if !v.nextIs(lexer.Operator) {
		return nil
	}

	op := stringToUnOpType(v.peek(0).Contents)
	if op == UNOP_ERR {
		return nil
	}
	startToken := v.consumeToken()

	value := v.parsePostfixExpr()
	if value == nil {
		// TODO: Restore this error once we go through with #655
		//v.err("Expected valid expression after unary operator")
		v.currentToken = startPos
		return nil
	}

	res := &UnaryExprNode{Value: value, Operator: op}
	res.SetWhere(lexer.NewSpan(startToken.Where.Start(), value.Where().End()))
	return res
}

// parseCompositeLiteral 解析结构体常量
// 实例：Person{id: 1, name: Name{first:"John",last:"Smith"}}
func (v *parser) parseCompositeLiteral() ParseNode {
	defer un(trace(v, "complit"))

	startPos := v.currentToken

	// 结构体常量以类型名称开头
	typ := v.parseTypeReference(true, true, true)

	// 内容以{开头
	if !v.tokenMatches(0, lexer.Separator, "{") {
		v.currentToken = startPos
		return nil
	}
	start := v.consumeToken() // eat opening bracket

	res := &CompositeLiteralNode{
		Type: typ,
	}

	var lastToken *lexer.Token

	// 循环解析每个成员
	for {
		// 遇到}结束
		if v.tokenMatches(0, lexer.Separator, "}") {
			lastToken = v.consumeToken()
			break
		}

		var field LocatedString

		// 解析成员名称，名称与值之间用:分隔
		if v.tokensMatch(lexer.Identifier, "", lexer.Operator, ":") {
			field = NewLocatedString(v.consumeToken())
			v.consumeToken()
		}

		// 解析成员的值。注意成员的值也可以是一个结构体常量
		val := v.parseCompositeLiteral()
		if val == nil { // 或者普通表达式
			val = v.parseExpr()
		}
		if val == nil {
			v.err("Expected value in composite literal, found `%s`", v.peek(0).Contents)
		}

		res.Fields = append(res.Fields, field)
		res.Values = append(res.Values, val)

		// 成员间以逗号分隔
		if v.tokenMatches(0, lexer.Separator, ",") {
			v.consumeToken()
			continue
		} else if v.tokenMatches(0, lexer.Separator, "}") {
			lastToken = v.consumeToken()
			break
		} else {
			v.err("Unexpected `%s`", v.peek(0).Contents)
		}
	}

	if typ != nil {
		res.SetWhere(lexer.NewSpan(typ.Where().Start(), lastToken.Where.End()))
	} else {
		res.SetWhere(lexer.NewSpanFromTokens(start, lastToken))
	}

	return res
}

// (expr, expr, expr)
func (v *parser) parseTupleLit() *TupleLiteralNode {
	defer un(trace(v, "tuplelit"))

	startPos := v.currentToken
	if !v.tokenMatches(0, lexer.Separator, "(") {
		return nil
	}
	startToken := v.consumeToken()

	var values []ParseNode
	for {
		if v.tokenMatches(0, lexer.Separator, ")") {
			break
		}

		value := v.parseExpr()
		if value == nil {
			// TODO: Restore this error once we go through with #655
			//v.err("Expected valid expression in tuple literal")
			v.currentToken = startPos
			return nil
		}
		values = append(values, value)

		if !v.tokenMatches(0, lexer.Separator, ",") {
			break
		}
		v.consumeToken()
	}

	endToken := v.peek(0)
	if !v.tokenMatches(0, lexer.Separator, ")") {
		// TODO: Restore this error once we go through wiht #655
		// endToken := v.expect(lexer.TOKEN_SEPARATOR, ")")
		v.currentToken = startPos
		return nil
	}
	v.currentToken++

	// Dirty hack
	if v.tokenMatches(0, lexer.Separator, ".") {
		v.currentToken = startPos
		return nil
	}

	res := &TupleLiteralNode{Values: values}
	res.SetWhere(lexer.NewSpanFromTokens(startToken, endToken))
	return res
}

// true/false
func (v *parser) parseBoolLit() *BoolLitNode {
	defer un(trace(v, "boollit"))

	if !v.tokenMatches(0, lexer.Identifier, "true") && !v.tokenMatches(0, lexer.Identifier, "false") {
		return nil
	}
	token := v.consumeToken()

	var value bool
	if token.Contents == "true" {
		value = true
	} else {
		value = false
	}

	res := &BoolLitNode{Value: value}
	res.SetWhere(token.Where)
	return res
}

// parseInt 解析base进制的整数
func parseInt(num string, base int) (*big.Int, bool) {
	// 支持_分隔，如 10000 可以写作 1_0000
	num = strings.ToLower(strings.Replace(num, "_", "", -1))

	// 根据e来分隔科学计数法中的基数和幂
	var splitNum []string
	if base == 10 {
		splitNum = strings.Split(num, "e")
	} else {
		splitNum = []string{num}
	}

	if !(len(splitNum) == 1 || len(splitNum) == 2) {
		return nil, false
	}

	numVal := splitNum[0]

	ret := big.NewInt(0)

	_, ok := ret.SetString(numVal, base)
	if !ok {
		return nil, false
	}

	// handle standard form
	if len(splitNum) == 2 {
		expVal := splitNum[1]

		exp := big.NewInt(0)
		_, ok = exp.SetString(expVal, base)
		if !ok {
			return nil, false
		}

		if exp.BitLen() > 64 {
			panic("TODO handle this better")
		}
		expInt := exp.Int64()

		ten := big.NewInt(10)

		if expInt < 0 {
			for ; expInt < 0; expInt++ {
				ret.Div(ret, ten)
			}
		} else if expInt > 0 {
			for ; expInt > 0; expInt-- {
				ret.Mul(ret, ten)
			}
		}
	}

	return ret, true
}

// parseNumberLit 解析数字常量，包括各个进制的整数、浮点数
func (v *parser) parseNumberLit() *NumberLitNode {
	defer un(trace(v, "numberlit"))

	if !v.nextIs(lexer.Number) {
		return nil
	}
	token := v.consumeToken()

	num := token.Contents
	var err error

	res := &NumberLitNode{}

	if strings.HasPrefix(num, "0x") || strings.HasPrefix(num, "0X") { // 十六进制
		ok := false
		res.IntValue, ok = parseInt(num[2:], 16)
		if !ok {
			v.errTokenSpecific(token, "Malformed hex literal: `%s`", num)
		}
	} else if strings.HasPrefix(num, "0b") { // 二进制
		ok := false
		res.IntValue, ok = parseInt(num[2:], 2)
		if !ok {
			v.errTokenSpecific(token, "Malformed binary literal: `%s`", num)
		}
	} else if strings.HasPrefix(num, "0o") { // 八进制
		ok := false
		res.IntValue, ok = parseInt(num[2:], 8)
		if !ok {
			v.errTokenSpecific(token, "Malformed octal literal: `%s`", num)
		}
	} else if lastRune := unicode.ToLower([]rune(num)[len([]rune(num))-1]); strings.ContainsRune(num, '.') || lastRune == 'f' || lastRune == 'd' || lastRune == 'q' { // 浮点数
		if strings.Count(num, ".") > 1 {
			v.errTokenSpecific(token, "Floating-point cannot have multiple periods: `%s`", num)
			return nil
		}
		res.IsFloat = true

		switch lastRune {
		case 'f', 'd', 'q':
			res.FloatSize = lastRune
		}

		if res.FloatSize != 0 {
			res.FloatValue, err = strconv.ParseFloat(num[:len(num)-1], 64)
		} else {
			res.FloatValue, err = strconv.ParseFloat(num, 64)
		}

		if err != nil {
			if err.(*strconv.NumError).Err == strconv.ErrSyntax {
				v.errTokenSpecific(token, "Malformed floating-point literal: `%s`", num)
			} else if err.(*strconv.NumError).Err == strconv.ErrRange {
				v.errTokenSpecific(token, "Floating-point literal cannot be represented: `%s`", num)
			} else {
				v.errTokenSpecific(token, "Unexpected error from floating-point literal: %s", err)
			}
		}
	} else { // 默认十进制整数
		ok := false
		res.IntValue, ok = parseInt(num, 10)
		if !ok {
			v.errTokenSpecific(token, "Malformed hex literal: `%s`", num)
		}
	}

	res.SetWhere(token.Where)
	return res
}

// parseStringLit 解析字符串常量。
func (v *parser) parseStringLit() *StringLitNode {
	defer un(trace(v, "stringlit"))

	var cstring bool
	var firstToken, stringToken *lexer.Token

	if v.tokenMatches(0, lexer.String, "") { // 普通字符串
		cstring = false
		firstToken = v.consumeToken()
		stringToken = firstToken
	} else if v.tokensMatch(lexer.Identifier, "c", lexer.String, "") { // c语言字符串：c"abc"
		cstring = true
		firstToken = v.consumeToken()
		stringToken = v.consumeToken()
	} else {
		return nil
	}

	// 读入代码中的字符串常量时，需要进行转义消解
	unescaped, err := UnescapeString(stringToken.Contents)
	if err != nil {
		v.errTokenSpecific(stringToken, "Invalid string literal: %s", err)
	}

	res := &StringLitNode{Value: unescaped, IsCString: cstring}
	res.SetWhere(lexer.NewSpan(firstToken.Where.Start(), stringToken.Where.End()))
	return res
}

// parseRuneLit 解析字符常量
func (v *parser) parseRuneLit() *RuneLitNode {
	defer un(trace(v, "runelit"))

	if !v.nextIs(lexer.Rune) {
		return nil
	}
	token := v.consumeToken()
	c, err := UnescapeString(token.Contents)
	if err != nil {
		v.errTokenSpecific(token, "Invalid character literal: %s", err)
	}

	res := &RuneLitNode{Value: []rune(c)[1]}
	res.SetWhere(token.Where)
	return res
}

func trace(v *parser, name string) *parser {
	v.pushRule(name)
	return v
}

func un(v *parser) {
	v.popRule()
}
