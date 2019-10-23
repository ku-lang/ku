package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/ku-lang/ku/ast"
	"github.com/ku-lang/ku/codegen"
	"github.com/ku-lang/ku/codegen/LLVMCodegen"
	"github.com/ku-lang/ku/doc"
	"github.com/ku-lang/ku/lexer"
	"github.com/ku-lang/ku/parser"
	"github.com/ku-lang/ku/semantic"
	"github.com/ku-lang/ku/util"
	"github.com/ku-lang/ku/util/log"
)

const (
	VERSION = "0.0.1"
	AUTHOR  = "zhaopuming"
)

var startTime time.Time

// 编译器程序入口
func main() {
	startTime = time.Now()

	// 利用kingpin库解析命令参数，详情参见args.go
	command := kingpin.MustParse(app.Parse(os.Args[1:]))
	log.SetLevel(*logLevel)
	log.SetTags(*logTags)

	// 初始化编译环境
	context := NewContext()

	// 解析命令
	switch command {
	case buildCom.FullCommand(): // build命令；编译代码
		// 下面这些变量均来自于args，从kingpin解析而来
		if *buildInput == "" {
			setupErr("No input files passed.")
		}

		context.Searchpaths = *buildSearchpaths
		context.Input = *buildInput

		outputType, err := codegen.ParseOutputType(*buildOutputType)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// 主流程：编译代码文件
		context.Build(*buildOutput, outputType, *buildCodegen, *buildOptLevel)

		printFinishedMessage(startTime, buildCom.FullCommand(), 1)

	case docgenCom.FullCommand(): // docgen命令：生成文档
		context.Searchpaths = *docgenSearchpaths
		context.Input = *docgenInput
		context.Docgen(*docgenDir)

		printFinishedMessage(startTime, docgenCom.FullCommand(), 1)
	}
}

func printFinishedMessage(startTime time.Time, command string, numFiles int) {
	dur := time.Since(startTime)
	log.Info("main", "%s (%d file(s), %.2fms)\n",
		util.TEXT_GREEN+util.TEXT_BOLD+fmt.Sprintf("Finished %s", command)+util.TEXT_RESET,
		numFiles, float32(dur.Nanoseconds())/1000000)
}

func setupErr(err string, stuff ...interface{}) {
	log.Error("main", util.TEXT_RED+util.TEXT_BOLD+"error:"+util.TEXT_RESET+" %s\n",
		fmt.Sprintf(err, stuff...))
	os.Exit(util.EXIT_FAILURE_SETUP)
}

// 类型：编译环境
type Context struct {
	// 搜索路径：所有搜索路径之下的.ku文件都会进行编译
	Searchpaths []string

	// 输入文件：待编译的主文件。现在只支持一个文件（通常是main.ku）
	Input string

	moduleLookup *ast.ModuleLookup
	depGraph     *ast.DependencyGraph
	modules      []*ast.Module

	modulesToRead []*ast.ModuleName
}

// 初始化编译环境
func NewContext() *Context {
	res := &Context{
		moduleLookup: ast.NewModuleLookup(""),
		depGraph:     ast.NewDependencyGraph(),
	}
	return res
}

// Build build a .ku source file
// 主流程：编译代码文件
func (v *Context) Build(output string, outputType codegen.OutputType, usedCodegen string, optLevel int) {
	// 首先加载runtime。注：其实这个加载过程也是一个完整的编译过程。
	runtimeModule := LoadRuntime()

	// 语法分析（其中也包含了词法分析），生成AST语法树
	v.parseFiles()

	// debug：打印parse的AST树
	for _, module := range v.modules {
		for _, submod := range module.Parts {
			// 打印AST
			log.Debugln("main", "AST of submodule `%s/%s`:", module.Name, submod.File.Name)
			for _, node := range submod.Nodes {
				log.Debugln("main", "%s", node.String())
			}
			log.Debugln("main", "")
		}
	}

	// 变量解析
	hasMainFunc := false
	log.Timed("resolve phase", "", func() {
		for _, module := range v.modules {
			ast.Resolve(module, v.moduleLookup)

			// Use module scope to check for main function
			mainIdent := module.ModScope.GetIdent(ast.UnresolvedName{Name: "main"})
			if mainIdent != nil && mainIdent.Type == ast.IDENT_FUNCTION && mainIdent.Public {
				hasMainFunc = true
			}
		}
	})

	// 如果没有找到主函数，直接退出
	if !hasMainFunc {
		log.Error("main", util.Red("error: ")+"main function not found\n")
		os.Exit(1)
	}

	// debug：打印parse的AST树
	for _, module := range v.modules {
		for _, submod := range module.Parts {
			// 打印AST
			log.Debugln("main", "AST of submodule `%s/%s`:", module.Name, submod.File.Name)
			for _, node := range submod.Nodes {
				log.Debugln("main", "%s", node.String())
			}
			log.Debugln("main", "")
		}
	}

	// 类型推导
	log.Timed("inference phase", "", func() {
		for _, module := range v.modules {
			for _, submod := range module.Parts {
				ast.Infer(submod)

				// 打印AST
				log.Debugln("main", "AST of submodule `%s/%s`:", module.Name, submod.File.Name)
				for _, node := range submod.Nodes {
					log.Debugln("main", "%s", node.String())
				}
				log.Debugln("main", "")
			}
		}
	})

	// 语义分析
	log.Timed("semantic analysis phase", "", func() {
		for _, module := range v.modules {
			semantic.SemCheck(module, *ignoreUnused)
		}
	})

	// 代码生成
	if usedCodegen != "none" {
		var gen codegen.Codegen

		// 现在后端只有llvm
		switch usedCodegen {
		case "llvm":
			gen = &LLVMCodegen.Codegen{
				OutputName: output,
				OutputType: outputType,
				OptLevel:   optLevel,
			}
		default:
			log.Error("main", util.Red("error: ")+"Invalid backend choice `"+usedCodegen+"`")
			os.Exit(1)
		}

		log.Timed("codegen phase", "", func() {
			mods := v.modules
			if runtimeModule != nil {
				mods = append(mods, runtimeModule)
			}
			gen.Generate(mods)
		})
	}
}

// Docgen 生成代码文档
func (v *Context) Docgen(dir string) {
	v.parseFiles()

	gen := &doc.Docgen{
		Input: v.modules,
		Dir:   dir,
	}

	gen.Generate()
}

// parseFiles 对各个文件进行分析。
// 分析过程包括：模块读取、文件读取、词法分析、语法分析、AST语法树构建
func (v *Context) parseFiles() {

	// 检查Input，如果是单个文件，就作为__main模块直接进行分析；如果是一个文件夹，建立对应的模块，并加入到待分析模块列表中
	if strings.HasSuffix(v.Input, ".ku") { // 如果输入是单个文件。只支持.ku文件名
		// 如果只有一个文件，则将它放入 __main 模块中
		modname := &ast.ModuleName{Parts: []string{"__main"}}
		module := &ast.Module{
			Name:    modname,
			Dirpath: "",
		}
		v.moduleLookup.Create(modname).Module = module

		// 直接分析该文件
		v.parseFile(v.Input, module)

		v.modules = append(v.modules, module)
	} else { // 如果输入是一个文件夹
		// 模块路径中不能包含'/', '.'和空格
		if strings.ContainsAny(v.Input, `\/. `) {
			setupErr("Invalid module name: %s", v.Input)
		}

		// 将整个文件作为一个模块加入待分析列表
		//modname := &ast.ModuleName{Parts: strings.Split(v.Input, "::")}
		modname := &ast.ModuleName{Parts: strings.Split(v.Input, ".")}
		v.modulesToRead = append(v.modulesToRead, modname)
	}

	// 读取所有待分析模块的文件，进行词法分析和语法分析
	log.Timed("read/lex/parse phase", "", func() {
		for i := 0; i < len(v.modulesToRead); i++ {
			modname := v.
				modulesToRead[i]

			// 如果模块已经读入，就不需要再次读入。
			if _, err := v.moduleLookup.Get(modname); err == nil {
				continue
			}

			// 找到模块对应的目录
			fi, dirpath, err := v.findModuleDir(modname.ToPath())
			if err != nil {
				setupErr("Couldn't find module `%s`: %s", modname, err)
			}

			if !fi.IsDir() {
				setupErr("Expected path `%s` to be directory, was file.", dirpath)
			}

			// 将模块加入到已处理模块组中。
			module := &ast.Module{
				Name:    modname,
				Dirpath: dirpath,
			}
			v.moduleLookup.Create(modname).Module = module

			// 检查模块下的各个文件
			childFiles, err := ioutil.ReadDir(dirpath)
			if err != nil {
				setupErr("%s", err.Error())
			}

			for _, childFile := range childFiles {
				// 忽略掉非.ku文件
				if strings.HasPrefix(childFile.Name(), ".") || !strings.HasSuffix(childFile.Name(), ".ku") {
					continue
				}

				actualFile := filepath.Join(dirpath, childFile.Name())

				// 对.ku文件进行分析（这个方法内部集成词法分析和语法分析）
				v.parseFile(actualFile, module)
			}

			// 当前模块处理结束，加入到编译环境中
			v.modules = append(v.modules, module)
		}
	})

	// 检查模块中的循环依赖
	log.Timed("cyclic dependency check", "", func() {
		errs := v.depGraph.DetectCycles()
		if len(errs) > 0 {
			log.Error("main", "%s: Encountered cyclic dependency between: ", util.Bold(util.Red("error")))
			for _, cycle := range errs {
				log.Error("main", "%s", cycle)
			}
			log.Errorln("main", "")
			os.Exit(util.EXIT_FAILURE_SETUP)
		}
	})

	// 构建AST语法树
	log.Timed("construction phase", "", func() {
		for _, module := range v.modules {
			ast.Construct(module, v.moduleLookup)
		}
	})
}

// parseFile 分析单个文件
func (v *Context) parseFile(path string, module *ast.Module) {
	// 读入文件内容
	sourcefile, err := lexer.NewSourcefile(path)
	if err != nil {
		setupErr("%s", err.Error())
	}

	// 进行词法分析（Lex），得到Token列表
	sourcefile.Tokens = lexer.Lex(sourcefile)

	// 进行语法分析（Parse），得到语法分析树。
	// 注：这里的语法分析树（ParseTree）与后面的 AST语法树 是不同的。之后的构建阶段（Construction）会根据语法分析树构建出AST语法树
	parseTree, deps := parser.Parse(sourcefile)
	module.Trees = append(module.Trees, parseTree)

	// Add dependencies to parse array
	for _, dep := range deps {
		depname := ast.NewModuleName(dep)
		v.modulesToRead = append(v.modulesToRead, depname)
		v.depGraph.AddDependency(module.Name, depname)

		if _, _, err := v.findModuleDir(depname.ToPath()); err != nil {
			log.Errorln("main", "%s [%s:%d:%d] Couldn't find module `%s`", util.Red("error:"),
				dep.Where().Filename, dep.Where().StartLine, dep.Where().EndLine,
				depname.String())
			log.Errorln("main", "%s", sourcefile.MarkSpan(dep.Where()))
			os.Exit(1)
		}
	}
}

// findModuleDir 搜寻模块目录
func (v *Context) findModuleDir(modulePath string) (fi os.FileInfo, path string, err error) {
	for _, searchPath := range v.Searchpaths {
		path := filepath.Join(searchPath, modulePath)
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		return fi, path, nil
	}

	return nil, "", fmt.Errorf("ku: Unable to find module `%s`", path)
}
