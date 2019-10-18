package parser

const (
	KEYWORD_AS        string = "as"
	KEYWORD_BREAK     string = "break"
	KEYWORD_C         string = "C"
	KEYWORD_DEFER     string = "defer"
	KEYWORD_DISCARD   string = "_"
	KEYWORD_DO        string = "do"
	KEYWORD_ELSE      string = "else"
	KEYWORD_ENUM      string = "enum"
	KEYWORD_FALSE     string = "false"
	KEYWORD_FOR       string = "for"
	KEYWORD_FUNC      string = "func"
	KEYWORD_FUN       string = "fun"
	KEYWORD_LEN       string = "len"
	KEYWORD_IF        string = "if"
	KEYWORD_MATCH     string = "match"
	KEYWORD_LET       string = "let"
	KEYWORD_VAR       string = "var"
	KEYWORD_CONTINUE  string = "continue"
	KEYWORD_PUB       string = "pub"
	KEYWORD_RETURN    string = "return"
	KEYWORD_SIZEOF    string = "sizeof"
	KEYWORD_STRUCT    string = "struct"
	KEYWORD_INTERFACE string = "interface"
	KEYWORD_TRUE      string = "true"
	KEYWORD_USE       string = "use"
	KEYWORD_VOID      string = "void"
)

var keywordList = []string{
	KEYWORD_AS,
	KEYWORD_BREAK,
	KEYWORD_C,
	KEYWORD_DEFER,
	KEYWORD_DISCARD,
	KEYWORD_DO,
	KEYWORD_ELSE,
	KEYWORD_ENUM,
	KEYWORD_FALSE,
	KEYWORD_FOR,
	KEYWORD_FUNC,
	KEYWORD_FUN,
	KEYWORD_LEN,
	KEYWORD_IF,
	KEYWORD_MATCH,
	KEYWORD_LET,
	KEYWORD_VAR,
	KEYWORD_CONTINUE,
	KEYWORD_PUB,
	KEYWORD_RETURN,
	KEYWORD_SIZEOF,
	KEYWORD_STRUCT,
	KEYWORD_INTERFACE,
	KEYWORD_TRUE,
	KEYWORD_USE,
	KEYWORD_VOID,
}

// Contains a map with all keywords as keys, and true as values
// Uses a map for quick lookup time when checking for reserved vars
var keywordMap map[string]bool

func init() {
	keywordMap = make(map[string]bool)

	for _, key := range keywordList {
		keywordMap[key] = true
	}
}

// 判断保留关键字
// Ark语言里，以下划线开头，并后接一个大写字母的变量，算作保留关键字。原因是这种变量名称有可能会与name mangling冲突
func IsReservedKeyword(s string) bool {
	if m := keywordMap[s]; m {
		return true
	}

	// names starting with a _ followed by an uppercase letter
	// are reserved as they can interfere with name mangling
	if len(s) >= 2 && s[0] == '_' && (s[1] >= 'A' && s[1] <= 'Z') {
		return true
	}

	return false
}
