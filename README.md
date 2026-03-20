# GlimmerWorksCli
帮助开发者自动生成代码并更新项目依赖版本。

### 打包构建
```
go build -o gwc
```

### 更新项目依赖
```
gwc updeps
```

### 代码生成器支持的注解
所有注解仅在生成 C++ 代码时生效。

```
//@genConfig
```
**必须定义在第一行**。代码生成器仅会在扫描到该注释后，才开始遍历文件内容。

```
//@genNextLine(shape|collision shape)
```
指示代码生成器自动解析下一行代码。

```
//@namespace(box2d)
```
定义命名空间。默认情况下，代码生成器会从命名空间声明语句中解析命名空间，因此无需显式指定。

```
//@include(filePath)
```
将指定文件路径（filePath）下的头文件内容写入输出文件。

```
//@content(index)
// [内容]
//@endContent
```
将包裹在该注解内的内容注入到代码体区域，并写入输出文件。
索引值会在生成的文件中按升序排列。

***

# GlimmerWorksCli

Helps developers automatically generate code and update project dependency versions.

### Packaging

```
go build -o gwc
```

### Update Project Dependencies

```
gwc updeps
```

### Annotations Available for Code Generator

All annotations are only valid when generating C++ code.

```
//@genConfig
```

MUST be defined on the first line. The code generator will start traversing file lines only after scanning this comment.
plaintext

```
//@genNextLine(shape|collision shape)
```

Instructs the code generator to automatically parse the next line of code.
plaintext

```
//@namespace(box2d)
```

Defines a namespace. By default, the code generator parses namespaces from namespace statements, so explicit specification is not required.
plaintext

```
//@include(filePath)
```

Writes the header file at filePath into the output file.
plaintext

```
//@content(index)
// [Content]
//@endContent
```

Injects the content into the body section and writes it to the output file.
The indices will be arranged in ascending order within the generated file.
