package lexer

import (
	"bytes"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ku-lang/ku/util"
)

// Sourcefile 源文件
type Sourcefile struct {
	Path     string   // 文件路径
	Name     string   // 文件名
	Contents []rune   // 文件内容
	NewLines []int    // 换行符列表
	Tokens   []*Token // 所有的词法符号
}

// NewSourcfile 根据文件路径，获取文件名，读入文件内容，并返回一个新的“源文件”对象
func NewSourcefile(filepath string) (*Sourcefile, error) {
	// TODO, get this to handle the rare //file//shit
	// cut out the filename from path
	// + 1 to cut out the slash.
	i, j := strings.LastIndex(filepath, "/")+1, strings.LastIndex(filepath, path.Ext(filepath))

	// this is the name of the file, not the path
	name := filepath[i:j]

	sf := &Sourcefile{Name: name, Path: filepath}
	sf.NewLines = append(sf.NewLines, -1)
	sf.NewLines = append(sf.NewLines, -1)

	contents, err := ioutil.ReadFile(sf.Path)
	if err != nil {
		return nil, err
	}

	sf.Contents = []rune(string(contents))
	return sf, nil
}

// GetLine 获取第line行内容，用于编译错误输出时打印错误对应的一行源码
func (s *Sourcefile) GetLine(line int) string {
	return string(s.Contents[s.NewLines[line]+1 : s.NewLines[line+1]])
}

// 默认的Tab宽度，用于错误输出
const TabWidth = 4

// MarkPos 标记一个位置，用于错误输出时，在错误行的错误位置下面显示^
func (s *Sourcefile) MarkPos(pos Position) string {
	buf := new(bytes.Buffer)

	lineString := s.GetLine(pos.Line)
	lineStringRunes := []rune(lineString)
	pad := pos.Char - 1

	buf.WriteString(strings.Replace(strings.Replace(lineString, "%", "%%", -1), "\t", "    ", -1))
	buf.WriteRune('\n')
	for i := 0; i < pad; i++ {
		spaces := 1

		if lineStringRunes[i] == '\t' {
			spaces = TabWidth
		}

		for t := 0; t < spaces; t++ {
			buf.WriteRune(' ')
		}
	}
	// 错误标记是绿色的粗体字，起到提示作用
	buf.WriteString(util.TEXT_GREEN + util.TEXT_BOLD + "^" + util.TEXT_RESET)
	buf.WriteRune('\n')

	return buf.String()

}

// MarkSpan 标记一段错误，需要打印多个^符号，即^^^^^^
func (s *Sourcefile) MarkSpan(span Span) string {
	// if the span is just one character, use MarkPos instead
	spanEnd := span.End()
	spanEnd.Char--
	if span.Start() == spanEnd {
		return s.MarkPos(span.Start())
	}

	// mark the span
	buf := new(bytes.Buffer)

	for line := span.StartLine; line <= span.EndLine; line++ {
		lineString := s.GetLine(line)
		lineStringRunes := []rune(lineString)

		var pad int
		if line == span.StartLine {
			pad = span.StartChar - 1
		} else {
			pad = 0
		}

		var length int
		if line == span.EndLine {
			length = span.EndChar - span.StartChar
		} else {
			length = len(lineStringRunes)
		}

		buf.WriteString(strings.Replace(strings.Replace(lineString, "%", "%%", -1), "\t", "    ", -1))
		buf.WriteRune('\n')

		for i := 0; i < pad; i++ {
			spaces := 1

			if lineStringRunes[i] == '\t' {
				spaces = TabWidth
			}

			for t := 0; t < spaces; t++ {
				buf.WriteRune(' ')
			}
		}

		buf.WriteString(util.TEXT_GREEN + util.TEXT_BOLD)
		for i := 0; i < length; i++ {
			// there must be a less repetitive way to do this but oh well
			spaces := 1

			if lineStringRunes[i+pad] == '\t' {
				spaces = TabWidth
			}

			for t := 0; t < spaces; t++ {
				buf.WriteRune('~')
			}
		}
		buf.WriteString(util.TEXT_RESET)
		buf.WriteRune('\n')
	}

	return buf.String()
}
