# 简介

喾语言（the Ku Programming Language，简称Kulang）是一个融合了Go、Rust、D、Kotlin的风格的静态语言。

“喾”取自于帝喾，他是中华始祖的三皇五帝之一，也是《山海经》中天帝帝俊的原型。

# 设计目标

1. 简洁：尽量减少代码冗余。二进制尺寸和运行时都尽量小。
2. 快速：接近C/C++的速度。
3. 开发效率：接近Go/D的开发效率。
3. 内存安全：内存管理机制参考rust，尽量保证内存安全。
4. 互操作：可以简单地调用C和Go语言的代码。

# 开发进度

当前版本：v0.0.1

已经实现的功能有：

- 变量定义（默认不可变，可使用var关键字定义可变变量）
- 函数定义
- 调用C语言函数（需要先用`[c]`标注来声明）
- 基于文件夹的模块化
- 自定义类型（类似Go语言的type struct），定义方法
- 接口，以及类似Go的接口实现方式
- 基本的流程控制和循环
- 基本的泛型支持
- 基本的运行时与标准库[lib](://github.com/ku-lang/lib)，未来会持续扩充

当前可运行的示例代码：

```go
// 引入标准库的io模块
use std::io

// 声明C语言外部函数
[C] fun printf(fmt ^u8, ...)  int;

// 最简单的函数定义
fun hello() {
  io::println("hello world")
}

// 定义有输入和输出参数的函数
fun add(a int, b int) int {
  return a + b
}

// 自定义类型
type Cat {
  name string
  age int
}

// 为类型定义方法
fun Cat.getAge() int {
  return this.age
}

// main函数
pub fun main() int {
  // 自定义函数调用 
  hello()

  // 标准库函数调用
  io::println("Hello, World!")

  // 直接调用C语言函数
  C::printf(c"%s,%s\n", c"abc", c"def")

  // let关键字用于声明一个值。喾语言的值默认是不可修改的。
  let a = 2
  // a = 3 // ERROR! value a is immutable
  io::printInt(add(a, 5))

  // var关键字用于声明一个变量。变量可以修改。
  var i = "abc"
  io::println(i)
  i = "def"
  io::println(i)

  // if语句
  if a > 1 {
    hello()
  }

  // 数组
  let xs = []int{1, 2, 3, 4}
  
  // for 循环。注：现在只支持最简单的for循环（相当于while循环），未来会加上 `for x in xs` 的形式
  var n = 0
  for n < len(xs) {
    io::println(xs[n])
    n += 1
  }

  // 自定义类型及方法
  let c  = Cat{name: "mew", age: 8}
  io::println(c.getAge())

  return 0

}
```

# 文档

喾语言相关的文档放在[ku-lang/docs](https://github.com/ku-lang/docs)中，包括以下几个文档：

- [0%] [语言设计](https://github.com/ku-lang/docs/blob/master/design/intro.md)
- [5%] [代码导读](https://github.com/ku-lang/docs/blob/master/coding/intro.md)
- [0%] [教程](https://github.com/ku-lang/docs/blob/master/tutorial/intro.md)
- [0%] [标准库](https://github.com/ku-lang/docs/blob/master/lib/std/intro.md)
- [0%] [书籍](https://github.com/ku-lang/docs/blob/master/book/intro.md)

# 近期计划

- [x] 将next关键字改为常用的continue
- [x] 增加let关键字，表示不可变值的声明。
- [x] 去掉变量的类型声明中的":"，改成类似Go语言的声明格式。即`var a: int`改为`var a int`；
- [x] 将C语言的标注从`[c]`改为`[C]`
- [x] 增加static关键字，用于定义类型内部的静态函数。
- [ ] 增加var static语句，用于定义类型内部的静态成员。
- [ ] 将模块访问符号`"::"`改为`"."`。由于结构成员访问符号也是`"."`，因此需要将`VariableAccessExpr`和`StructAccessExpr`合并起来，并处理对应的Resolve/Inference环节。
- [x] 修改方法定义格式，不再使用类似Go的格式，而是使用类似Kotlin的格式，即`fun Student.sayHello()`
- [x] 配合上一条，增加this关键字，用来表示当前对象。
- [ ] 弄清楚为什么不把CompositeLiteral直接放到Expr中，而是每次都单独判断。换个说法：结构体常量是不是一个表达式？
- [ ] 深入阅读Ark编译器的代码，理清流程，添加注释，写出一个编译器设计文档。
- [ ] 实现`for i in range`。
- [x] 去掉自定义类型定义中的struct关键字。直接 `type Book { title string }` 即可。即type定义的默认类型是struct
- [ ] 增加对字符串内联的支持。如"Hello $world!"
- [ ] 可变参数。类似Go/D的varargs，去掉对C风格varargs的支持，或者限制其只在C交互块中使用。
- [ ] 实现io::println()的可变参数版本
- [ ] iterator/yield
- [ ] 增加对JSON的支持。即语言内置 `[1, 2, 3]`形式的数组，以及 `{key: value, key: value}` 形式的对象。可能要去掉`[]int{1, 2, 3}` 这种形式。
- [ ] 在lex过程中保留必要的换行符，而不是全部忽略。在语法分析中，应当判断语句结束时的换行符。

# 中期计划

- [ ] 2019-11: v0.1, 完成对ark语言代码的研读；完成ku语言的设计文档。对ark编译器进行小规模语法修改，使之贴近ku语言。 
- [ ] 2019-12: v0.2, 完成基本的语法修改，将现有的ark语法全部转变为ku语言语法。完成两个ku语言独有功能：字符串内嵌功能；类似D/Go的可变参数。
- [ ] 2020-01: v0.3, 实现类似Go的fmt.Printf函数。扩展io.print模块。丰富其他std基本模块，如arrays, strings等。
- [ ] 2020-02: v0.4, 实现基本的builder功能。包括：运行时导入；标准库导入；基本的增量编译；链接；运行；多个执行目标；
- [ ] 2020-03: v0.5, 实现net库，可以创建tcp/udp的服务器与客户端程序
- [ ] 2020-04: v0.6, 实现http/web库，可以建立一个http/web服务。
- [ ] 2020-05: v0.7, 实现基本的项目依赖管理功能。并利用http/web库搭建一个喾语言项目仓库。
- [ ] 2020-06: v0.8, 实现debug相关的功能

至此，ku语言基本到达可以发布的状况。

# 鸣谢

喾语言编译器ku的初始实现根源自[Ark编程语言](https://github.com/ark-lang/ark)，特此鸣谢。

Ark编译器的LICENSE文件参见[LICENSE_ARK](LICENSE_ARK)。