package main

import (
	"github.com/ku-lang/ku/ast"
	"github.com/ku-lang/ku/lexer"
	"github.com/ku-lang/ku/parser"
	"github.com/ku-lang/ku/semantic"
)

// TODO: Move this at a file and handle locating/specifying this file
const RuntimeSource = `
[C] fun printf(fmt ^u8, ...) int;
[C] fun exit(code C::int);

pub fun panic(message string) {
	if len(message) == 0 {
		C::printf(c"\n")
	} else {
		C::printf(c"panic: %.*s\n", len(message), &message[0])
	}
	C::exit(-1)
}

pub type Option enum<T> {
    Some(T),
    None,
}

pub fun (o Option<T>) unwrap() T {
    match o {
        Some(t) => return t,
        None => panic("Option.unwrap: expected Some, have None"),
    }

    let a T
    return a
}

type RawArray struct {
    size uint,
    ptr uintptr,
}

pub fun makeArray<T>(ptr ^T, size uint) []T {
	let raw = RawArray{size: size, ptr: uintptr(ptr)}
	return @(^[]T)(uintptr(^raw))
}

pub fun breakArray<T>(arr []T) (uint, ^T) {
	let raw = @(^RawArray)(uintptr(^arr))
	return (raw.size, (^T)(raw.ptr))
}
`

// LoadRuntime 加载运行时
func LoadRuntime() *ast.Module {
	runtimeModule := &ast.Module{
		Name: &ast.ModuleName{
			Parts: []string{"__runtime"},
		},
		Dirpath: "__runtime",
		Parts:   make(map[string]*ast.Submodule),
	}

	// 注：这里没有读取runtime.ku文件，而是直接写在代码中。
	sourcefile := &lexer.Sourcefile{
		Name:     "runtime",
		Path:     "runtime.ku",
		Contents: []rune(RuntimeSource),
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
