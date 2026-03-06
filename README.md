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
//@genCode
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
