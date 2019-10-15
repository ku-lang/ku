package lexer

// Note that most of the panic() calls should be removed once the lexer is bug-free.

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/ku-lang/ku/util/log"

	"github.com/ku-lang/ku/util"
)

// lexer 词法分析器
type lexer struct {
	input            *Sourcefile // 输入文件
	startPos, endPos int         // 在分析过程中用来定位每个Token在代码字符串中的起始和结束位置
	curPos           Position    // 当前位置
	tokStart         Position    // token的开始位置
}

// errPos 输出错误信息，打印错误位置，并退出程序
func (v *lexer) errPos(pos Position, err string, stuff ...interface{}) {
	log.Errorln("lexer", util.TEXT_RED+util.TEXT_BOLD+"error:"+util.TEXT_RESET+" [%s:%d:%d] %s",
		pos.Filename, pos.Line, pos.Char, fmt.Sprintf(err, stuff...))

	log.Error("lexer", v.input.MarkPos(pos))

	os.Exit(1)
}

// err errPos的语法糖
func (v *lexer) err(err string, stuff ...interface{}) {
	v.errPos(v.curPos, err, stuff...)
}

// peek 提前窥看ahead个字节，但分析器并不前进，这些字节仍然可以继续进行其他分析
func (v *lexer) peek(ahead int) rune {
	if ahead < 0 {
		panic(fmt.Sprintf("Tried to peek a negative number: %d", ahead))
	}

	if v.endPos+ahead >= len(v.input.Contents) {
		return 0
	}
	return v.input.Contents[v.endPos+ahead]
}

// consume 消化一个字符。当分析器分析过一个字符，并转化为token之后，调用该函数，前进一步，不再需要这个字符了
func (v *lexer) consume() {
	v.curPos.Char++
	if v.peek(0) == '\n' {
		v.curPos.Char = 1
		v.curPos.Line++
		v.input.NewLines = append(v.input.NewLines, v.endPos)
	}

	v.endPos++
}

// expect 期待一个字符r。如果接下来的字符与r不一致，则报错并退出。
// 常用于词法结构中的固定搭配判断。
func (v *lexer) expect(r rune) {
	if v.peek(0) == r {
		v.consume()
	} else {
		v.err("Expected `%c`, found `%c`", r, v.peek(0))
	}
}

// discardBuffer 抛弃当前缓存的结果，从当前位置开始新的解析。
// 用于完成一次解析后清空buffer，以免影响下一次解析
func (v *lexer) discardBuffer() {
	v.startPos = v.endPos

	v.tokStart = v.curPos
}

// pushToken 将t加入到Tokens列表中。
// 解析出一个token之后调用此方法。
func (v *lexer) pushToken(t TokenType) {
	tok := &Token{
		Type:     t,
		Contents: string(v.input.Contents[v.startPos:v.endPos]),
		Where:    NewSpan(v.tokStart, v.curPos),
	}

	v.input.Tokens = append(v.input.Tokens, tok)

	// 输出当前token。在Debug模式下，可以通过这个输出看到词法分析器获取的所有token列表。
	log.Debug("lexer", "[%4d:%4d:% 11s] `%s`\n", v.startPos, v.endPos, tok.Type, tok.Contents)

	// 清空缓存，以便继续分析
	v.discardBuffer()
}

// Lex 词法分析的主函数。对input源文件进行词法分析，并返回一个Token数组
func Lex(input *Sourcefile) []*Token {
	// 创建一个词法分析器实例，具体参数的作用，参见lexer类型的声明注释
	l := &lexer{
		input:    input,
		startPos: 0,
		endPos:   0,
		curPos:   Position{Filename: input.Name, Line: 1, Char: 1},
		tokStart: Position{Filename: input.Name, Line: 1, Char: 1},
	}

	// 调用lex()方法开始词法分析
	log.Timed("lexing", input.Name, func() {
		l.lex()
	})

	// 词法分析结束后，从lexer.input.Tokens可以获取分析到的Token列表
	return l.input.Tokens
}

// lex 词法分析器的主功能方法。
func (v *lexer) lex() {
	// 词法分析循环，探测下一个字符，并根据它的具体情况来识别不同类型的Token
	for {
		// 首先需要跳过空白或注释
		v.skipLayoutAndComments()

		// 如果遇到文件结尾(EOF)，跳出循环并返回
		if isEOF(v.peek(0)) {
			v.input.NewLines = append(v.input.NewLines, v.endPos)
			return
		}

		if isDecimalDigit(v.peek(0)) { // 十进制数字
			v.recognizeNumberToken()
		} else if isLetter(v.peek(0)) || v.peek(0) == '_' { // 变量标识：以字母或'_'开头
			v.recognizeIdentifierToken()
		} else if v.peek(0) == '"' { // 字符串
			v.recognizeStringToken()
		} else if v.peek(0) == '\'' { // 字符
			v.recognizeCharacterToken()
		} else if isOperator(v.peek(0)) { // 操作符号
			v.recognizeOperatorToken()
		} else if isSeparator(v.peek(0)) { // 分隔符号
			v.recognizeSeparatorToken()
		} else { // 所有其他的字符都是非法的
			v.err("Unrecognised token")
		}
	}
}

// skipComment 跳过注释，如果遇到并跳过了注释，返回值是true；如果没有遇到注释，返回false
// returns true if a comment was skipped
func (v *lexer) skipComment() bool {
	if v.skipBlockComment() { // 先判断块注释
		return true
	} else if v.skipLineComment() { // 再判断行内注释
		return true
	} else {
		return false
	}
}

// skipBlockComment 跳过块注释。块注释是 /* ... */ 结构。
// 注：这里支持块注释的多层嵌套，即  /* ... /* ... */ ... */ 的形式
func (v *lexer) skipBlockComment() bool {
	pos := v.curPos

	// 块注释以 "/*" 开头
	if v.peek(0) != '/' || v.peek(1) != '*' {
		return false
	}
	// 跳过 '/' 和 '*' 字符
	v.consume()
	v.consume()

	// 如果还有一个 '*' ，即  "/**" 形式，则该注释块是文档注释
	isDoc := v.peek(0) == '*'

	// 记录注释嵌套深度，以支持多层注释嵌套
	depth := 1
	for depth > 0 { // 当嵌套深度减为0时，正好匹配到上面的开始符号。因此这个块注释结束，跳出循环。
		// 嵌套深度大于1时遇到文件结尾，说明注释结束符号与开始符号不匹配
		if isEOF(v.peek(0)) {
			v.errPos(pos, "Unterminated block comment")
		}

		// 如果中途遇到注释开始符号 "/*"，则注释嵌套深度加1.
		if v.peek(0) == '/' && v.peek(1) == '*' {
			v.consume()
			v.consume()
			depth += 1
		}

		// 如果遇到注释结束符号 "*/"，则注释嵌套深度减1.
		if v.peek(0) == '*' && v.peek(1) == '/' {
			v.consume()
			v.consume()
			depth -= 1
		}

		// 其他所有字符，直接消耗掉
		v.consume()
	}

	if isDoc { // 如果是文档注释，仍然返回一个类型为Doccomment的Token
		v.pushToken(Doccomment)
	} else { // 其他注释则直接跳过
		v.discardBuffer()
	}
	return true
}

// 跳过单行注释
func (v *lexer) skipLineComment() bool {
	// 单行注释以 "//" 开头
	if v.peek(0) != '/' || v.peek(1) != '/' {
		return false
	}
	v.consume()
	v.consume()

	// 如果还有一个 '/'，即 "///" 开头，则为单行文档注释
	isDoc := v.peek(0) == '/'

	// 循环跳过之后所有字符，直到当前行结束，或文件结束
	for {
		// isEOL:当前行结束；isEOF:代码文件结束
		if isEOL(v.peek(0)) || isEOF(v.peek(0)) {
			if isDoc {
				v.pushToken(Doccomment)
			} else {
				v.discardBuffer()
			}
			v.consume()
			return true
		}
		v.consume()
	}
}

// skipLayoutAndComments 跳过空白和注释
func (v *lexer) skipLayoutAndComments() {
	// 循环判断空白或注释，两者都不是的时候，表示接下来是需要识别的内容，跳出循环
	for {
		// 如果下面是空白，直接抛弃掉
		for isLayout(v.peek(0)) {
			v.consume()
		}
		v.discardBuffer()

		// 如果是注释，跳过
		if !v.skipComment() {
			break
		}
	}

	//v.printBuffer()
	// 清空缓存，准备开始真正的词法识别
	v.discardBuffer()
}

func (v *lexer) lexNumberWithValidator(validator func(rune) bool) {
	for {
		if validator(v.peek(0)) || v.peek(0) == '_' {
			v.consume()
		} else if v.peek(0) == 'e' || v.peek(0) == 'E' {
			v.consume()
			if v.peek(0) == '-' {
				v.consume()
			}
		} else {
			v.pushToken(Number)
			return
		}
	}
}

// recognizeNumberToken 识别数字符号
// 喾语言支持的数字符号包括：
// 1. 十六进制（Hexadecimal）：0x12AF 0X334B
// 2. 二进制（Binary）：0b0001
// 3. 八进制（Octal）：0o127
// 4. 十进制（Decimal)：12345
// 5. 32位浮点数（float/f32）：1234.56f
// 6. 64位浮点数（double/f64）：1234.56d
// 7. 128位浮点数（f128）：1234.56q
func (v *lexer) recognizeNumberToken() {
	// 由于调用该函数前已经判断了 isDecimalDigit，因此可以先消耗掉第一个数字字符
	v.consume()

	// 检查第2个字符
	if v.peek(0) == 'x' || v.peek(0) == 'X' { // 如果是 'x' 或 'X'，则表示是 0x12AF 这种类型的十六进制数字
		// Hexadecimal
		v.consume()
		// 检查剩余的数字是否符合十六进制
		v.lexNumberWithValidator(isHexDigit)
	} else if v.peek(0) == 'b' {
		// Binary
		v.consume()
		// 检查剩余的数字是否符合二进制
		v.lexNumberWithValidator(isBinaryDigit)
	} else if v.peek(0) == 'o' {
		// Octal
		v.consume()
		// 检查剩余的数字是否符合八进制
		v.lexNumberWithValidator(isOctalDigit)
	} else { // 如果第二个字符也是数字，则该数字是十进制或浮点数
		// Decimal or floating
		v.lexNumberWithValidator(func(r rune) bool {
			if isDecimalDigit(r) || r == '.' {
				return true
			}
			peek := unicode.ToLower(r)
			return peek == 'f' || peek == 'd' || peek == 'q'
		})
	}
}

// recognizeIdentifierToken 识别标识符
// 注：标识符由字母、数字或下划线组成，并且不能以数字开头，以方便与数字Token进行区分
// 注：这里的字母指的是Unicode字母，因此标识符也支持类似 "你好" 这样的中文字符
func (v *lexer) recognizeIdentifierToken() {
	// 消耗掉之前判断的第一个字符
	v.consume()

	// 如果下一个字符是Unicode字母（isLetter）、数字（isDecimalDigit）或 '_' ，则继续前进
	for isLetter(v.peek(0)) || isDecimalDigit(v.peek(0)) || v.peek(0) == '_' {
		v.consume()
	}

	// 如果遇到其他字符，表示当前标识符结束了
	v.pushToken(Identifier)
}

// recognizeStringToken 识别字符串
func (v *lexer) recognizeStringToken() {
	pos := v.curPos

	// 消耗开始的 " 字符
	v.expect('"')
	v.discardBuffer()

	for {
		if v.peek(0) == '\\' && v.peek(1) == '"' { // 跳过转义的 \" 字符，以支持字符串内存储 "
			v.consume()
			v.consume()
		} else if v.peek(0) == '"' { // 如果再遇到一个"字符，则表示字符串结束
			v.pushToken(String)
			v.consume()
			return
		} else if isEOF(v.peek(0)) { // 如果还没遇到结束"字符，就遇到文件结尾，则是词法错误
			v.errPos(pos, "Unterminated string literal")
		} else { // 跳过其他字符
			v.consume()
		}
	}
}

// recognizeCharaterToken 识别单个字符
// 注：除了普通的单字符，如'A'这样的情况，还有其他几种情况：
// 1. Unicode字符，如'中'
// 2. 转义字符，如'\b', '\''
func (v *lexer) recognizeCharacterToken() {
	pos := v.curPos

	// 消耗开始的'字符
	v.expect('\'')

	// 如果下一个字符也是'，则这是一个空字符。喾语言不允许空字符，抛出词法错误
	if v.peek(0) == '\'' {
		v.err("Empty character constant")
	}

	for {
		// 处理转义字符 \\ 和 \'
		if v.peek(0) == '\\' && (v.peek(1) == '\'' || v.peek(1) == '\\') {
			v.consume()
			v.consume()
		} else if v.peek(0) == '\'' { // 遇到另一个‘时，字符结束，返回Rune类型的结果
			v.consume()
			v.pushToken(Rune)
			return
		} else if isEOF(v.peek(0)) { // 如果没有遇到另一个'就到了文件末尾，则是词法错误
			v.errPos(pos, "Unterminated character literal")
		} else { // 接收其他字符
			v.consume()
		}
	}
}

// recognizeOperatorToken 识别操作符
func (v *lexer) recognizeOperatorToken() {
	if strings.ContainsRune("=!><", v.peek(0)) && v.peek(1) == '=' { // 双字符操作符：==, !=, >=, <=
		v.consume()
		v.consume()
	} else if v.peek(0) == '>' && v.peek(1) == '>' { // 连续两个 >>，是不同的操作符。用于支持嵌套泛型
		v.consume()
	} else { // 两个连续的操作符识别为混合操作符。TODO：why？
		v.consume()
		// never consume @, ^ or = into an mixed/combined operator
		// TODO: why?
		if isOperator(v.peek(0)) && !strings.ContainsRune("@^=", v.peek(0)) {
			v.consume()
		}
	}

	// 操作符最长只能是两个字符
	v.pushToken(Operator)
}

// recognizeSeparatorToken 识别分隔符
func (v *lexer) recognizeSeparatorToken() {
	// 分隔符不需要做判断，直接加入到Token列表即可
	v.consume()
	v.pushToken(Separator)
}

// 辅助函数

func isDecimalDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isHexDigit(r rune) bool {
	return isDecimalDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isBinaryDigit(r rune) bool {
	return r == '0' || r == '1'
}

func isOctalDigit(r rune) bool {
	return r >= '0' && r <= '7'
}

func isLetter(r rune) bool {
	return unicode.IsLetter(r)
}

func isOperator(r rune) bool {
	return strings.ContainsRune("+-*/=><!~?:|&%^#@", r)
}

func isSeparator(r rune) bool {
	return strings.ContainsRune(" ;,.`(){}[]", r)
}

func isEOL(r rune) bool {
	return r == '\n'
}

func isEOF(r rune) bool {
	return r == 0
}

func isLayout(r rune) bool {
	return (r <= ' ' || unicode.IsSpace(r)) && !isEOF(r)
}
