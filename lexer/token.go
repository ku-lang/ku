package lexer

// TokenType Token类型
type TokenType int

const (
	Rune       TokenType = iota // 单个字符
	Identifier                  // 标识符，用于标识变量、类型等
	Separator                   // 分隔符
	Operator                    // 操作符
	Number                      // 数字
	Erroneous                   // 错误的词法类型
	String                      // 字符串
	Doccomment                  // 文档注释
)

var tokenStrings = []string{"rune", "identifier", "separator", "operator", "number", "erroneous", "string", "doccomment"}

// 打印TokenType实例对应的名称
func (v TokenType) String() string {
	return tokenStrings[v]
}

// Token 词号，用来存放词法分析的结果。每个记号对应源码中的一个词法单位。词法分析返回一个词号列表，用于下一步的语法分析。
type Token struct {
	Type     TokenType // 词号类型
	Contents string    // 内容
	Where    Span      // 位置范围
}

// Position 单个字符的位置：文件、行、第几个字符
type Position struct {
	Filename string

	Line, Char int
}

// Span 一段字符串的位置范围：文件、开始行、开始字符、结束行、结束字符。用来记录较长的、可能跨行的词号，比如文档注释；或者用于记录多个词号对应的位置，用于编译器错误输出。
type Span struct {
	Filename string

	StartLine, StartChar int
	EndLine, EndChar     int
}

// NewSpan 根据start和end两个Position对象新建一个Span
func NewSpan(start, end Position) Span {
	return Span{Filename: start.Filename,
		StartLine: start.Line, StartChar: start.Char,
		EndLine: end.Line, EndChar: end.Char,
	}
}

// 从两个Token对象中构造出一个范围
func NewSpanFromTokens(start, end *Token) Span {
	return Span{Filename: start.Where.Filename,
		StartLine: start.Where.StartLine, StartChar: start.Where.StartChar,
		EndLine: end.Where.EndLine, EndChar: end.Where.EndChar,
	}
}

// 获取Span的开始位置
func (s Span) Start() Position {
	return Position{Filename: s.Filename,
		Line: s.StartLine, Char: s.StartChar}
}

// 获取Span的结束位置
func (s Span) End() Position {
	return Position{Filename: s.Filename,
		Line: s.EndLine, Char: s.EndChar}
}
