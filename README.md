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

### 代码生成器可用的注解。

所有的注解仅在生成c++代码时可用。

```
//@genCode
```

必须放在第一行定义。代码生成器扫描到这个注释后，才会遍历文件行。

```
//@genNextLine(shape|碰撞形状)
```

让代码生成器自动分析下一行代码。

```
//@namespace(box2d)
```
定义命名空间。默认情况下代码生成器会从namespace语句内解析，无需显式指定。

```
//@include(filePath)
```
将位于filePath的头文件，写入到输出文件内。

```
//@content
//【内容】
//@endContent
```
将内容注入到body部分，写入到输出文件内。

