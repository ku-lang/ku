package main

import (
	"io/ioutil"

	"github.com/ku-lang/ku/ast"
	"github.com/ku-lang/ku/lexer"
	"github.com/ku-lang/ku/parser"
	"github.com/ku-lang/ku/semantic"
)

// LoadRuntime 加载运行时
func LoadRuntime() *ast.Module {
	runtimeModule := &ast.Module{
		Name: &ast.ModuleName{
			Parts: []string{"__runtime"},
		},
		Dirpath: "__runtime",
		Parts:   make(map[string]*ast.Submodule),
	}

	// TODO: 从配置文件里读取runtime.ku的路径
	runtimePath := "/usr/local/ku/lib/runtime.ku"
	bytes, err := ioutil.ReadFile(runtimePath)
	if err != nil {
		panic("INIT ERROR: Cannot load runtime.ku in " + runtimePath)
	}
	sourcefile := &lexer.Sourcefile{
		Name:     "runtime",
		Path:     "runtime.ku",
		Contents: []rune(string(bytes)),
		NewLines: []int{-1, -1},
	}

	// 先进行词法分析，得到一个token列表
	lexer.Lex(sourcefile)

	// 接着进行语法分析，生产一个AST语法树
	tree, deps := parser.Parse(sourcefile)
	if len(deps) > 0 {
		panic("INTERNAL ERROR: No dependencies allowed in runtime")
	}
	// 每个模块一个语法树
	runtimeModule.Trees = append(runtimeModule.Trees, tree)

	// 构建每个模块的语法树
	ast.Construct(runtimeModule, nil)

	// 解析各个变量是否合法
	ast.Resolve(runtimeModule, nil)

	// 对语法树进行类型推导
	for _, submod := range runtimeModule.Parts {
		ast.Infer(submod)
	}

	// 进行语义检查
	semantic.SemCheck(runtimeModule, *ignoreUnused)

	// 最有把运行时模块加载到ast中
	ast.LoadRuntimeModule(runtimeModule)

	return runtimeModule
}
