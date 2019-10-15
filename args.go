package main

import (
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// 利用kinpin库解析编译器参数
var (
	app = kingpin.New("ark", "Compiler for the Ku programming language.").Version(VERSION).Author(AUTHOR)

	// log参数。log的实现参见util/log/log.go`
	logLevel = app.Flag("loglevel", "Set the level of logging to show").Default("info").Enum("debug", "verbose", "info", "warning", "error")
	// TODO: 当前日志标签是分散写在编译器各个文件中的，没有统一收集。需要收集起来做成常量或enum，并在命令行信息中展示。
	logTags = app.Flag("logtags", "Which log tags to show").Default("all").String()

	// 命令：build。
	buildCom         = app.Command("build", "Build an executable.")
	buildOutput      = buildCom.Flag("output", "Output binary name.").Short('o').Default("main").String()
	buildSearchpaths = buildCom.Flag("searchpaths", "Paths to search for used modules if not found in base directory").Short('I').Strings()
	buildInput       = buildCom.Arg("input", "Ku source file or package").String()
	buildCodegen     = buildCom.Flag("codegen", "Codegen backend to use").Default("llvm").Enum("none", "llvm")
	buildOutputType  = buildCom.Flag("output-type", "The format to produce after code generation").Default("executable").Enum("executable", "assembly", "object", "llvm-ir")
	buildOptLevel    = buildCom.Flag("opt-level", "LLVM optimization level").Short('O').Default("0").Int()
	ignoreUnused     = buildCom.Flag("unused", "Do not error on unused declarations").Bool()

	// 命令：docgen。生成文档。
	docgenCom         = app.Command("docgen", "Generate documentation.")
	docgenDir         = docgenCom.Flag("dir", "Directory to place generated docs in.").Default("docgen").String()
	docgenInput       = docgenCom.Arg("input", "Ku source file or package").String()
	docgenSearchpaths = docgenCom.Flag("searchpaths", "Paths to search for used modules if not found in base directory").Short('I').Strings()
)
