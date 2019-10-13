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

# 鸣谢

喾语言编译器ku的初始实现参考了[Ark编程语言](https://github.com/ark-lang/ark)，特此鸣谢。

Ark编译器的LICENSE文件是[LICENSE_ARK](LICENSE_ARK)。
