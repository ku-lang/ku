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

当前可运行的示例代码：

```ku
use std::io

[c] fun printf(fmt: ^u8, ...)  int;

fun hello() {
  io::println("hello")
}

fun add(a: int, b: int) int {
  return a + b
}

pub fun main() int {
  // call std::io::println
  io::println("Hello, World!")

  // call a user defined function
  hello()

  // a var is mutable
  var i := "abc"
  io::println(i)
  i = "def"
  io::println(i)

  a := 2
  io::printInt(add(a, 5))
  if a > 1 {
    hello()
  }

  b := 5; c:= 6
  io::printInt(add(b, c))
  io::println("")

  // call C functions directly
  C::printf(c"%s,%s\n", c"abc", c"def")

  return 0
}
```

# 近期计划

- [ ] 将模块访问符号"::"改为"."。由于结构成员访问符号也是"."，因此需要将VariableAccessExpr和StructAccessExpr合并起来，并处理对应的Resolve/Inference环节。
- [ ] 去掉变量的类型声明中的":"，改成类似Go语言的声明格式。即`var a: int`改为`var a int`；
- [ ] 增加let关键字，表示不可变值的声明。
- [ ] 修改方法定义格式，不再使用类似Go的格式，而是使用类似Kotlin的格式，即`fun Student.sayHello()`
- [ ] 配合上一条，增加this关键字，用来表示当前对象。
- [ ] 去掉自定义类型定义中的struct关键字。直接 `type Book { title string }` 即可。
- [ ] 增加对字符串内联的支持
- [ ] 可变参数。类似Go/D的varargs，去掉对C风格varargs的支持，或者限制其只在C交互块中使用。
- [ ] 实现io::println()的可变参数版本
- [ ] iterator/range

# 鸣谢

喾语言编译器ku的初始实现参考了[Ark编程语言](https://github.com/ark-lang/ark)，特此鸣谢。

Ark编译器的LICENSE文件是[LICENSE_ARK](LICENSE_ARK)。
