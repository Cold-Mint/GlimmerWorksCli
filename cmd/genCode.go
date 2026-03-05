/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"GlimmerWorksCli/meta"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// 匹配类/结构体定义行（优先处理）
	// 支持：struct Resource { / struct StringResource : Resource { / class ResourceRefArg {
	classStructDefRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*(?::\s*[a-zA-Z0-9_\s]+)?\{`)
	// 提取继承关系：struct A : B { → 子类A，父类B（忽略public/private/protected）
	inheritanceExtractRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*:\s*(?:public|private|protected)?\s*([a-zA-Z0-9_]+)\s*\{`)
	// 匹配字段行（支持复杂类型+默认值）
	// 支持：std::string name; / std::vector<MobAppearanceResource> appearance; / uint32_t argType_ = XXX;
	fieldRegex = regexp.MustCompile(`^((?:[a-zA-Z0-9_:]+)(?:<.*>)?)+\s+([a-zA-Z0-9_]+)\s*(=\s*([^;]+))?;`)
	// 匹配//@genNextLine(注解内容)，提取括号内的全部内容（支持|分隔的中英文）
	genNextLineNoteRegex = regexp.MustCompile(`^//@genNextLine\((.+)\)$`)
	// 新增：解析//@namespace(xxx)注解
	namespaceAnnotationRegex = regexp.MustCompile(`^//@namespace\((.+)\)$`)
	// 新增：解析namespace xxx { 代码行
	namespaceCodeRegex = regexp.MustCompile(`^namespace\s+([a-zA-Z0-9_]+)\s*\{`)
)

// parseClassInfo 解析类/结构体定义行，返回类名和父类名
func parseClassInfo(line string) (className, parentClassName string) {
	// 先提取继承关系
	if inheritMatches := inheritanceExtractRegex.FindStringSubmatch(line); inheritMatches != nil {
		return inheritMatches[2], inheritMatches[3]
	}
	// 无继承的基础定义
	if classMatches := classStructDefRegex.FindStringSubmatch(line); classMatches != nil {
		return classMatches[2], ""
	}
	return "", ""
}

// parseGenNextLineNote 解析//@genNextLine标记行，提取括号内的注解内容
func parseGenNextLineNote(line string) string {
	matches := genNextLineNoteRegex.FindStringSubmatch(line)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// processGenCodeFile 完全重构：先解析所有类定义，再处理标记
func processGenCodeFile(outPutFilePath string, filePath string, fieldMetas *[]meta.FieldMeta) {
	relativePath, err := filepath.Rel(outPutFilePath, filePath)
	if err != nil {
		fmt.Printf("Failed to get relative path for %s: %v\n", filePath, err)
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Failed to open file %s: %v\n", filePath, err)
		return
	}
	defer file.Close()

	// 第一步：全量读取文件内容（解决行顺序问题）
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("Failed to read file %s: %v\n", filePath, err)
		return
	}

	// 第二步：检查首行是否为//@genCode
	if len(lines) == 0 || lines[0] != "//@genCode" {
		return
	}
	fmt.Printf("=== Processing file: %s ===\n", filePath)

	// 第三步：预解析所有类/结构体的位置和名称（构建类名映射）
	type classInfo struct {
		name         string
		parentName   string
		startLineIdx int // 类定义开始行（索引）
		endLineIdx   int // 类定义结束行（索引，匹配}）
	}
	var classList []classInfo
	currentClass := classInfo{}
	braceCount := 0

	for idx, line := range lines {
		// 匹配类定义开始
		if classStructDefRegex.MatchString(line) {
			if currentClass.name != "" {
				// 之前的类未结束，先标记结束
				currentClass.endLineIdx = idx - 1
				classList = append(classList, currentClass)
			}
			// 解析新类
			cn, pn := parseClassInfo(line)
			currentClass = classInfo{
				name:         cn,
				parentName:   pn,
				startLineIdx: idx,
				endLineIdx:   len(lines) - 1, // 默认到文件末尾
			}
			braceCount = 1 // 类定义行有一个{
		}

		// 统计大括号，确定类结束位置
		if currentClass.name != "" {
			braceCount += strings.Count(line, "{")
			braceCount -= strings.Count(line, "}")
			if braceCount == 0 {
				currentClass.endLineIdx = idx
				classList = append(classList, currentClass)
				currentClass = classInfo{}
			}
		}
	}
	var namespaceMarks []meta.NamespaceMark
	var currentFileNamespace string // 兜底的全局命名空间（代码行解析的）

	// 遍历所有行，收集命名空间标记
	for lineIdx, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 1. 解析//@namespace(xxx)注解（优先级最高）
		if matches := namespaceAnnotationRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			ns := strings.TrimSpace(matches[1])
			if ns != "" && !strings.HasSuffix(ns, "::") {
				ns += "::"
			}
			namespaceMarks = append(namespaceMarks, meta.NamespaceMark{
				LineIdx:   lineIdx,
				Namespace: ns,
			})
			continue
		}

		// 2. 解析namespace xxx {代码行（作为兜底）
		if matches := namespaceCodeRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			ns := strings.TrimSpace(matches[1])
			if ns != "" && !strings.HasSuffix(ns, "::") {
				ns += "::"
			}
			currentFileNamespace = ns // 全局兜底命名空间
			// 也可以加入标记列表（如果需要）
			namespaceMarks = append(namespaceMarks, meta.NamespaceMark{
				LineIdx:   lineIdx,
				Namespace: ns,
			})
			continue
		}
	}

	// 第四步：遍历所有//@genNextLine标记，解析字段和注解
	for lineIdx, line := range lines {
		// 匹配标记行
		if !strings.HasPrefix(line, "//@genNextLine(") {
			continue
		}

		// 提取标记行的注解内容
		note := parseGenNextLineNote(line)

		// 标记行的下一行索引
		fieldLineIdx := lineIdx + 1
		if fieldLineIdx >= len(lines) {
			fmt.Printf("Line %d: Found genNextLine mark but reached end of file\n", lineIdx+1)
			continue
		}
		fieldLine := lines[fieldLineIdx]
		fmt.Printf("Line %d: Found genNextLine mark [Note: %s], field line: %s\n", lineIdx+1, note, fieldLine)

		// 先检查下一行是否是类定义行（更新类名映射）
		cn, _ := parseClassInfo(fieldLine)
		if cn != "" {
			// 是类定义行，跳过字段解析（避免无效提示）
			continue
		}

		// 匹配字段行
		fieldMatches := fieldRegex.FindStringSubmatch(fieldLine)
		if fieldMatches == nil {
			fmt.Printf("Line %d: Invalid field format, skip: %s\n", fieldLineIdx+1, fieldLine)
			continue
		}

		// 找到当前字段所属的类
		var currentClassName, currentParentName string
		for _, ci := range classList {
			if fieldLineIdx >= ci.startLineIdx && fieldLineIdx <= ci.endLineIdx {
				currentClassName = ci.name
				currentParentName = ci.parentName
				break
			}
		}

		// 新增核心逻辑：根据字段行号，找到当前生效的命名空间
		var currentNamespace string
		// 遍历所有命名空间标记，找到「字段行号之前最后一个」标记
		for _, nm := range namespaceMarks {
			if nm.LineIdx < fieldLineIdx { // 标记在行号之前才生效
				currentNamespace = nm.Namespace
			} else {
				break // 后续标记行号更大，无需继续
			}
		}
		// 兜底：如果没有注解标记，使用代码行解析的全局命名空间
		if currentNamespace == "" {
			currentNamespace = currentFileNamespace
		}

		// 提取字段信息
		fieldType := strings.TrimSpace(fieldMatches[1])
		// 补全字段类型的命名空间（自定义类型）
		if !strings.Contains(fieldType, "::") {
			switch fieldType {
			case "bool", "int", "uint32_t", "uint64_t", "float", "uint8_t", "size_t", "std::string":
				break // 基础类型不补全
			default:
				fieldType = currentNamespace + fieldType
			}
		}
		// 处理std::vector<XXX>类型的命名空间补全
		if strings.HasPrefix(fieldType, "std::vector<") {
			// 提取vector内部类型（去掉std::vector<和>）
			innerType := strings.TrimSuffix(strings.TrimPrefix(fieldType, "std::vector<"), ">")
			if !strings.Contains(innerType, "::") {
				innerType = currentNamespace + innerType
			}
			fieldType = "std::vector<" + innerType + ">"
		}

		fieldName := strings.TrimSpace(fieldMatches[2])
		fieldDefault := strings.TrimSpace(fieldMatches[4])

		// 补全类名和父类名的命名空间
		if currentClassName != "" && !strings.Contains(currentClassName, "::") {
			currentClassName = currentNamespace + currentClassName
		}
		if currentParentName != "" && !strings.Contains(currentParentName, "::") {
			currentParentName = currentNamespace + currentParentName
		}

		// 添加到FieldMeta（使用当前生效的命名空间）
		*fieldMetas = append(*fieldMetas, meta.FieldMeta{
			RelativePath:    relativePath,
			ClassName:       currentClassName,
			ParentClassName: currentParentName,
			Type:            fieldType,
			Name:            fieldName,
			Default:         fieldDefault,
			Note:            note,
		})
	}

	fmt.Println("=== Processing completed ===\n")
}

// generateCPPHeaderFile 生成匹配示例格式的C++头文件
func generateCPPHeaderFile(outputPath string, fieldMetas []meta.FieldMeta) error {
	var headerContent strings.Builder
	headerContent.WriteString("// Auto-generated by GlimmerWorksCli\n")
	headerContent.WriteString("// Do not edit manually!\n\n")
	headerContent.WriteString("#pragma once\n\n")
	headerContent.WriteString("#include \"toml11/find.hpp\"\n")

	var bodyContent strings.Builder
	classFields := make(map[string][]meta.FieldMeta)
	pathSet := make(map[string]struct{})
	for _, fm := range fieldMetas {
		if fm.ClassName == "" {
			continue
		}
		if fm.RelativePath != "" {
			pathSet[fm.RelativePath] = struct{}{}
		}
		classFields[fm.ClassName] = append(classFields[fm.ClassName], fm)
	}
	if len(pathSet) > 0 {
		headerContent.WriteString("// Include original header files\n")
		for path := range pathSet {
			headerContent.WriteString(fmt.Sprintf("#include \"%s\"\n", path))
		}
		headerContent.WriteString("\n\n")
	}
	for className, fields := range classFields {
		bodyContent.WriteString("    template<>\n")
		bodyContent.WriteString("    struct from<")
		bodyContent.WriteString(className)
		bodyContent.WriteString("> {\n")
		bodyContent.WriteString("        static ")
		bodyContent.WriteString(className)
		bodyContent.WriteString(" from_toml(const value &v) {\n")
		bodyContent.WriteString("            ")
		bodyContent.WriteString(className)
		bodyContent.WriteString(" r;\n")
		for _, fm := range fields {
			bodyContent.WriteString("            r.")
			bodyContent.WriteString(fm.Name)
			bodyContent.WriteString(" = toml::find<")
			bodyContent.WriteString(fm.Type)
			bodyContent.WriteString(">")

			bodyContent.WriteString("(v, \"")
			bodyContent.WriteString(fm.Name)
			bodyContent.WriteString("\");\n")
		}
		bodyContent.WriteString("            return r;\n")
		bodyContent.WriteString("        }\n")
		bodyContent.WriteString("    };\n\n")
	}
	bodyContent.WriteString("}\n")
	filePath := filepath.Join(outputPath, "FieldMeta.gen.h")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create C++ header file: %v", err)
	}
	defer file.Close()
	_, err = file.WriteString(headerContent.String() + bodyContent.String())
	if err != nil {
		return fmt.Errorf("failed to write C++ header file: %v", err)
	}

	fmt.Printf("Successfully generated C++ header file: %s\n", filePath)
	return nil
}

// generateJSONFile 生成JSON元信息文件
func generateJSONFile(outputPath string, fieldMetas []meta.FieldMeta) error {
	// 美化JSON输出
	data, err := json.MarshalIndent(fieldMetas, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}

	// 创建输出文件
	file, err := os.Create(filepath.Join(outputPath, "field_meta.gen.json"))
	if err != nil {
		return fmt.Errorf("failed to create json file: %v", err)
	}
	defer file.Close()

	// 写入文件
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write json file: %v", err)
	}

	fmt.Printf("Successfully generated JSON file: %s\n", filepath.Join(outputPath, "field_meta.gen.json"))
	return nil
}

// genCodeCmd 优化输出格式，新增文件生成功能
var genCodeCmd = &cobra.Command{
	Use:   "genCode",
	Short: "Parse C++ files with //@genCode annotation and generate FieldMeta",
	Long:  `Parse all .cpp/.h files with //@genCode annotation, extract class/struct field info (type/name/default/note) and output FieldMeta/C++ Header/JSON files.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Printf("Failed to get current directory: %v\n", err)
			return
		}

		// 解析命令行参数
		outputPath, _ := cmd.Flags().GetString("outputPath")
		outputType, _ := cmd.Flags().GetInt8("outputType")

		// 设置默认输出路径
		if outputPath == "" {
			outputPath = dir
		}

		// 验证输出路径是否存在
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			// 创建目录
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				fmt.Printf("Failed to create output directory: %v\n", err)
				return
			}
		}

		var fieldMetas []meta.FieldMeta
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("Failed to access file: %v\n", err)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".cpp" && ext != ".h" {
				return nil
			}

			processGenCodeFile(outputPath, path, &fieldMetas)
			return nil
		})

		// 输出所有FieldMeta信息
		fmt.Println("=== All FieldMeta Information ===")
		if len(fieldMetas) == 0 {
			fmt.Println("No FieldMeta found.")
		} else {
			// 表头（调整列宽，新增Note列）
			fmt.Printf("%-4s %-30s %-25s %-20s %-30s %-20s %-20s %s\n",
				"NO", "RelativePath", "Class", "Parent Class", "Type", "Name", "Default", "Note")
			fmt.Println(strings.Repeat("-", 180))
			// 内容
			for i, fm := range fieldMetas {
				parent := fm.ParentClassName
				if parent == "" {
					parent = "-"
				}
				fmt.Printf("%-4d %-30s %-25s %-20s %-30s %-20s %-20s %s\n",
					i+1, fm.RelativePath, fm.ClassName, parent, fm.Type, fm.Name, fm.Default, fm.Note)
			}
		}

		// 根据outputType生成文件
		switch outputType {
		case 1:
			// 生成C++头文件（匹配示例格式）
			if err := generateCPPHeaderFile(outputPath, fieldMetas); err != nil {
				fmt.Printf("Failed to generate C++ header file: %v\n", err)
			}
		case 2:
			// 生成JSON文件
			if err := generateJSONFile(outputPath, fieldMetas); err != nil {
				fmt.Printf("Failed to generate JSON file: %v\n", err)
			}
		case 0:
			// 不生成文件
			fmt.Println("=== No file generation (outputType=0) ===")
		default:
			fmt.Printf("Invalid outputType: %d (0=none, 1=CPP header, 2=JSON meta info)\n", outputType)
		}

		if err != nil {
			fmt.Printf("Failed to traverse directory: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(genCodeCmd)
	// 文件输出路径，如果为空，那么设置为os.Getwd()
	genCodeCmd.Flags().StringP("outputPath", "o", "", "File output path (default: current directory)")
	// 0为不输出，1为输出cpp头文件，2为输出json文件。默认0
	genCodeCmd.Flags().Int8P("outputType", "t", 0, "Output type (0=none, 1=CPP header (TomlUtils.h), 2=JSON meta info)")
}
