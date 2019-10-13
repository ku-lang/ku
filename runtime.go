package main

import (
	"github.com/ku-lang/ku/ast"
	"github.com/ku-lang/ku/lexer"
	"github.com/ku-lang/ku/parser"
	"github.com/ku-lang/ku/semantic"
)

// TODO: Move this at a file and handle locating/specifying this file
const RuntimeSource = `
#[c] fun printf(fmt: ^u8, ...) int;
#[c] fun exit(code: C::int);

pub fun panic(message: string) {
	if len(message) == 0 {
		C::printf(c"\n");
	} else {
		C::printf(c"panic: %.*s\n", len(message), &message[0]);
	}
    C::exit(-1);
}

pub type Option enum<T> {
    Some(T),
    None,
};

pub fun (o: Option<T>) unwrap() T {
    match o {
        Some(t) => return t,
        None => panic("Option.unwrap: expected Some, have None"),
    }

    a: T;
    return a;
}

type RawArray struct {
    size: uint,
    ptr: uintptr,
};

pub fun makeArray<T>(ptr: ^T, size: uint) []T {
	raw := RawArray{size: size, ptr: uintptr(ptr)};
	return @(^[]T)(uintptr(^raw));
}

pub fun breakArray<T>(arr: []T) (uint, ^T) {
	raw := @(^RawArray)(uintptr(^arr));
	return (raw.size, (^T)(raw.ptr));
}
`

func LoadRuntime() *ast.Module {
	runtimeModule := &ast.Module{
		Name: &ast.ModuleName{
			Parts: []string{"__runtime"},
		},
		Dirpath: "__runtime",
		Parts:   make(map[string]*ast.Submodule),
	}

	sourcefile := &lexer.Sourcefile{
		Name:     "runtime",
		Path:     "runtime.ku",
		Contents: []rune(RuntimeSource),
		NewLines: []int{-1, -1},
	}
	lexer.Lex(sourcefile)

	tree, deps := parser.Parse(sourcefile)
	if len(deps) > 0 {
		panic("INTERNAL ERROR: No dependencies allowed in runtime")
	}
	runtimeModule.Trees = append(runtimeModule.Trees, tree)

	ast.Construct(runtimeModule, nil)
	ast.Resolve(runtimeModule, nil)

	for _, submod := range runtimeModule.Parts {
		ast.Infer(submod)
	}

	semantic.SemCheck(runtimeModule, *ignoreUnused)

	ast.LoadRuntimeModule(runtimeModule)

	return runtimeModule
}
