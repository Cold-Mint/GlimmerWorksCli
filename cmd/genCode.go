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
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// 匹配类/结构体定义行（优先处理）
	// 支持：struct Resource { / struct StringResource : Resource { / class ResourceRefArg {
	classStructDefRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*(?::\s*[a-zA-Z0-9_\s]+)?\{`)
	// 提取继承关系：struct A : B { → 子类A，父类B（忽略public/private/protected）
	inheritanceExtractRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*:\s*(?:public|private|protected)?\s*([a-zA-Z0-9_]+)\s*\{`)
	// 匹配//@genNextLine(注解内容)，提取括号内的全部内容（支持|分隔的中英文）
	genNextLineNoteRegex = regexp.MustCompile(`^//@genNextLine\((.+)\)$`)
	// 新增：解析//@namespace(xxx)注解
	namespaceAnnotationRegex = regexp.MustCompile(`^//@namespace\((.+)\)$`)
	// 新增：解析namespace xxx { 代码行
	namespaceCodeRegex = regexp.MustCompile(`^namespace\s+([a-zA-Z0-9_]+)\s*\{`)
	// 匹配//@include(filePath)注解
	includeAnnotationRegex = regexp.MustCompile(`^//@include\((.+)\)$`)
	// 匹配//@content开始标记
	contentStartRegex = regexp.MustCompile(`^//@content$`)
	// 匹配//@endContent结束标记
	contentEndRegex = regexp.MustCompile(`^//@endContent$`)
	fieldRegex      = regexp.MustCompile(`^((?:[a-zA-Z0-9_:]+)(?:<.*>)?)+\s+([a-zA-Z0-9_]+)\s*(=\s*([^;]+))?;`)
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

func processGenCodeFile(outPutFilePath string, filePath string, fieldMetas *[]meta.FieldMeta, extraMeta *meta.FileExtraMeta) error {
	relativePath, err := filepath.Rel(outPutFilePath, filePath)
	if err != nil {
		fmt.Printf("Failed to get relative path for %s: %v\n", filePath, err)
		return err
	}
	// 恢复原有的文件打开、按行读取逻辑
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read file failed: %v", err)
	}

	// 恢复：检查首行是否为//@genCode，非目标文件直接返回
	if len(lines) == 0 || lines[0] != "//@genCode" {
		return nil
	}

	// ========== 新增：解析@include和@content注解 ==========
	var inContentBlock bool // 标记是否在//@content块内
	var currentContent string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 解析//@include(filePath)
		if matches := includeAnnotationRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			incPath := strings.TrimSpace(matches[1])
			if incPath != "" {
				extraMeta.IncludePaths = append(extraMeta.IncludePaths, incPath)
			}
			continue
		}

		// 解析//@content开始标记
		if contentStartRegex.MatchString(trimmedLine) {
			inContentBlock = true
			currentContent = ""
			continue
		}

		// 解析//@endContent结束标记
		if contentEndRegex.MatchString(trimmedLine) {
			inContentBlock = false
			trimmedContent := strings.TrimSpace(currentContent)
			if trimmedContent != "" { // 仅保留非空内容
				extraMeta.ContentBlocks = append(extraMeta.ContentBlocks, currentContent)
			}
			continue
		}

		// 收集content块内的内容
		if inContentBlock {
			currentContent += line + "\n"
		}
	}

	var classList []meta.ClassInfo
	currentClass := meta.ClassInfo{}
	braceCount := 0
	for idx, line := range lines {
		if classStructDefRegex.MatchString(line) {
			if currentClass.Name != "" {
				currentClass.EndLineIdx = idx - 1
				classList = append(classList, currentClass)
			}
			cn, pn := parseClassInfo(line)
			currentClass = meta.ClassInfo{
				Name:         cn,
				ParentName:   pn,
				StartLineIdx: idx,
				EndLineIdx:   len(lines) - 1,
			}
			braceCount = 1
		}

		if currentClass.Name != "" {
			braceCount += strings.Count(line, "{")
			braceCount -= strings.Count(line, "}")
			if braceCount == 0 {
				currentClass.EndLineIdx = idx
				classList = append(classList, currentClass)
				currentClass = meta.ClassInfo{}
			}
		}
	}
	var namespaceMarks []meta.NamespaceMark
	var currentFileNamespace string
	for lineIdx, line := range lines {
		trimmedLine := strings.TrimSpace(line)
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
		if matches := namespaceCodeRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			ns := strings.TrimSpace(matches[1])
			if ns != "" && !strings.HasSuffix(ns, "::") {
				ns += "::"
			}
			currentFileNamespace = ns
			namespaceMarks = append(namespaceMarks, meta.NamespaceMark{
				LineIdx:   lineIdx,
				Namespace: ns,
			})
			continue
		}
	}
	for lineIdx, line := range lines {
		if !strings.HasPrefix(line, "//@genNextLine(") {
			continue
		}
		note := parseGenNextLineNote(line)
		fieldLineIdx := lineIdx + 1
		if fieldLineIdx >= len(lines) {
			fmt.Printf("Line %d: genNextLine mark but end of file\n", lineIdx+1)
			continue
		}
		fieldLine := lines[fieldLineIdx]
		cn, _ := parseClassInfo(fieldLine)
		if cn != "" {
			continue
		}
		fieldMatches := fieldRegex.FindStringSubmatch(fieldLine)
		if fieldMatches == nil {
			fmt.Printf("Line %d: invalid field format: %s\n", fieldLineIdx+1, fieldLine)
			continue
		}
		var currentClassName, currentParentName string
		for _, ci := range classList {
			if fieldLineIdx >= ci.StartLineIdx && fieldLineIdx <= ci.EndLineIdx {
				currentClassName = ci.Name
				currentParentName = ci.ParentName
				break
			}
		}
		var currentNamespace string
		for _, nm := range namespaceMarks {
			if nm.LineIdx < fieldLineIdx {
				currentNamespace = nm.Namespace
			} else {
				break
			}
		}
		if currentNamespace == "" {
			currentNamespace = currentFileNamespace
		}
		fieldType := strings.TrimSpace(fieldMatches[1])
		if !strings.Contains(fieldType, "::") {
			switch fieldType {
			case "bool", "int", "uint32_t", "uint64_t", "float", "uint8_t", "size_t", "std::string":
				break
			default:
				fieldType = currentNamespace + fieldType
			}
		}
		if strings.HasPrefix(fieldType, "std::vector<") {
			innerType := strings.TrimSuffix(strings.TrimPrefix(fieldType, "std::vector<"), ">")
			if !strings.Contains(innerType, "::") {
				innerType = currentNamespace + innerType
			}
			fieldType = "std::vector<" + innerType + ">"
		}
		fieldName := strings.TrimSpace(fieldMatches[2])
		fieldDefault := strings.TrimSpace(fieldMatches[4])
		if currentClassName != "" && !strings.Contains(currentClassName, "::") {
			currentClassName = currentNamespace + currentClassName
		}
		if currentParentName != "" && !strings.Contains(currentParentName, "::") {
			currentParentName = currentNamespace + currentParentName
		}
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
	return nil
}

// generateCPPHeaderFile 生成匹配示例格式的C++头文件
func generateCPPHeaderFile(outPutFilePath string, fieldMetas []meta.FieldMeta, includePaths []string, contentBlocks []string) error {
	var headerContent strings.Builder
	headerContent.WriteString("// Auto-generated by GlimmerWorksCli\n")
	headerContent.WriteString("// Do not edit manually!\n\n")
	headerContent.WriteString("#pragma once\n\n")

	// 注入//@include的头文件
	if len(includePaths) > 0 {
		headerContent.WriteString("// Injected by //@include annotations\n")
		for _, incPath := range includePaths {
			headerContent.WriteString(fmt.Sprintf("#include \"%s\"\n", incPath))
		}
		headerContent.WriteString("\n")
	}

	// 注入原有的头文件引用
	var pathSet = make(map[string]struct{})
	for _, fm := range fieldMetas {
		if fm.RelativePath != "" {
			pathSet[fm.RelativePath] = struct{}{}
		}
	}
	if len(pathSet) > 0 {
		headerContent.WriteString("// Original header files\n")
		for path := range pathSet {
			headerContent.WriteString(fmt.Sprintf("#include \"%s\"\n", path))
		}
		headerContent.WriteString("\n")
	}

	// 注入//@content的内容块
	if len(contentBlocks) > 0 {
		headerContent.WriteString("// Injected by //@content annotations\n")
		for _, content := range contentBlocks {
			headerContent.WriteString(content)
			headerContent.WriteString("\n")
		}
		headerContent.WriteString("\n")
	}

	var bodyContent strings.Builder
	classFields := make(map[string][]meta.FieldMeta)
	for _, fm := range fieldMetas {
		if fm.ClassName == "" {
			continue
		}
		classFields[fm.ClassName] = append(classFields[fm.ClassName], fm)
	}

	// 1. 提取类依赖关系
	depsMap := extractClassDependencies(classFields)
	// 2. 拓扑排序（被依赖的类优先）
	sortedClasses := topologicalSort(depsMap)

	// 3. 按拓扑排序后的顺序生成代码
	bodyContent.WriteString("namespace toml {\n\n")
	for _, className := range sortedClasses {
		fields := classFields[className]
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

	// 合并内容
	headerContent.WriteString(bodyContent.String())

	// 生成文件（仅执行一次）
	filePath := filepath.Join(outPutFilePath, "TomlUtils.h")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file failed: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(headerContent.String())
	if err != nil {
		return fmt.Errorf("write file failed: %v", err)
	}

	fmt.Printf("Successfully generated: %s\n", filePath)
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

// extractClassDependencies 提取所有类的依赖关系
func extractClassDependencies(classFields map[string][]meta.FieldMeta) map[string]meta.ClassDependency {
	depsMap := make(map[string]meta.ClassDependency)
	// 匹配自定义类的正则（排除基础类型/std::xxx）
	basicTypeRegex := regexp.MustCompile(`^(bool|int|uint8_t|uint32_t|uint64_t|float|size_t|std::string|std::vector<.+>)$`)
	// 提取vector内部类型的正则
	vectorRegex := regexp.MustCompile(`^std::vector<(.+)>$`)

	// 第一步：收集所有类名
	allClasses := make(map[string]bool)
	for className := range classFields {
		allClasses[className] = true
	}

	// 第二步：分析每个类的依赖
	for className, fields := range classFields {
		deps := make(map[string]bool) // 去重存储依赖
		for _, fm := range fields {
			fieldType := fm.Type
			// 跳过基础类型
			if basicTypeRegex.MatchString(fieldType) {
				continue
			}
			// 解析vector内部类型
			if matches := vectorRegex.FindStringSubmatch(fieldType); len(matches) >= 2 {
				fieldType = matches[1]
			}
			// 检查是否是自定义类（存在于allClasses中）
			if allClasses[fieldType] {
				deps[fieldType] = true
			}
		}
		// 转换为切片
		depList := make([]string, 0, len(deps))
		for dep := range deps {
			depList = append(depList, dep)
		}
		depsMap[className] = meta.ClassDependency{
			ClassName: className,
			Deps:      depList,
		}
	}
	return depsMap
}

// topologicalSort 对类进行拓扑排序（被依赖的类优先）
func topologicalSort(depsMap map[string]meta.ClassDependency) []string {
	// 1. 构建入度表和邻接表
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	// 初始化
	for className := range depsMap {
		inDegree[className] = 0
		adj[className] = []string{}
	}
	// 填充入度和邻接表
	for className, dep := range depsMap {
		for _, d := range dep.Deps {
			adj[d] = append(adj[d], className) // d -> className（d被className依赖）
			inDegree[className]++              // className的入度+1
		}
	}

	// 2. 拓扑排序（Kahn算法）
	var queue []string
	// 入度为0的类（无依赖）先入队
	for className, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, className)
		}
	}

	// 3. 处理队列
	var sortedClasses []string
	for len(queue) > 0 {
		// 取出队首元素
		current := queue[0]
		queue = queue[1:]
		sortedClasses = append(sortedClasses, current)
		// 处理当前类的邻接节点
		for _, neighbor := range adj[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// 4. 检查是否有环（理论上C++类无环，此处仅容错）
	if len(sortedClasses) != len(depsMap) {
		fmt.Println("Warning: 检测到类依赖环，使用原始顺序兜底")
		// 兜底：返回原始类名排序
		sortedClasses = make([]string, 0, len(depsMap))
		for className := range depsMap {
			sortedClasses = append(sortedClasses, className)
		}
		sort.Strings(sortedClasses)
	}

	return sortedClasses
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
		var allIncludePaths []string  // 收集所有文件的include路径
		var allContentBlocks []string // 收集所有文件的content内容块

		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("access file failed: %v\n", err)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			// 仅处理指定后缀的文件（如.h/.cpp，可根据需要调整）
			if !strings.HasSuffix(path, ".h") && !strings.HasSuffix(path, ".cpp") {
				return nil
			}

			// 解析当前文件：收集字段和注解
			var extraMeta meta.FileExtraMeta
			if err := processGenCodeFile(outputPath, path, &fieldMetas, &extraMeta); err != nil {
				fmt.Printf("process file %s failed: %v\n", path, err)
				return err
			}

			// 合并当前文件的注解到全局
			includeSet := make(map[string]struct{})
			for _, path := range extraMeta.IncludePaths {
				if path != "" {
					includeSet[path] = struct{}{}
				}
			}
			for path := range includeSet {
				allIncludePaths = append(allIncludePaths, path)
			}
			contentSet := make(map[string]struct{})
			for _, content := range extraMeta.ContentBlocks {
				trimmed := strings.TrimSpace(content)
				if trimmed != "" {
					contentSet[content] = struct{}{}
				}
			}
			for content := range contentSet {
				allContentBlocks = append(allContentBlocks, content)
			}
			return nil
		})

		if err != nil {
			fmt.Printf("walk dir failed: %v\n", err)
			return
		}

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
			// 生成C++头文件（传递include和content参数）
			if err := generateCPPHeaderFile(outputPath, fieldMetas, allIncludePaths, allContentBlocks); err != nil {
				fmt.Printf("generate header failed: %v\n", err)
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
