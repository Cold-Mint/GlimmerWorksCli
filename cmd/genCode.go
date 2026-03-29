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
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// 匹配类/结构体定义行（优先处理）
	classStructDefRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*(?::\s*[a-zA-Z0-9_\s]+)?\{`)
	// 提取继承关系
	inheritanceExtractRegex = regexp.MustCompile(`^(class|struct)\s+([a-zA-Z0-9_]+)\s*:\s*(?:public|private|protected)?\s*([a-zA-Z0-9_]+)\s*\{`)
	// 匹配//@genNextLine注解
	genNextLineNoteRegex = regexp.MustCompile(`^//@genNextLine\((.+)\)$`)
	// 解析//@namespace注解
	namespaceAnnotationRegex = regexp.MustCompile(`^//@namespace\((.+)\)$`)
	// 解析namespace代码行
	namespaceCodeRegex = regexp.MustCompile(`^namespace\s+([a-zA-Z0-9_]+)\s*\{`)
	// 匹配//@include注解
	includeAnnotationRegex = regexp.MustCompile(`^//@include\((.+)\)$`)
	// 匹配//@content(index)开始标记（强制数字索引）
	contentStartRegex = regexp.MustCompile(`^//@content\((\d+)\)$`)
	// 匹配//@endContent结束标记
	contentEndRegex = regexp.MustCompile(`^//@endContent$`)
	// 字段匹配正则
	fieldRegex = regexp.MustCompile(`^([a-zA-Z0-9_:]+(?:<.*>)?)+\s+([a-zA-Z0-9_]+)\s*(=\s*([^;]+))?;`)
)

func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var result strings.Builder
	result.WriteByte(s[0])
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result.WriteByte('_')
			result.WriteByte(c + 32)
		} else {
			result.WriteByte(c)
		}
	}
	return strings.ToLower(result.String())
}

// parseClassInfo 解析类/结构体定义行，返回类名和父类名
func parseClassInfo(line string) (className, parentClassName string) {
	if inheritMatches := inheritanceExtractRegex.FindStringSubmatch(line); inheritMatches != nil {
		return inheritMatches[2], inheritMatches[3]
	}
	if classMatches := classStructDefRegex.FindStringSubmatch(line); classMatches != nil {
		return classMatches[2], ""
	}
	return "", ""
}

// parseGenNextLineNote 解析//@genNextLine标记行，提取注解内容
func parseGenNextLineNote(line string) string {
	matches := genNextLineNoteRegex.FindStringSubmatch(line)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// removeLineComments 移除内容中每行开头的//注释符号
func removeLineComments(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var result strings.Builder
	commentRegex := regexp.MustCompile(`^\s*//`)

	for scanner.Scan() {
		line := scanner.Text()
		cleanLine := commentRegex.ReplaceAllString(line, "")
		result.WriteString(cleanLine + "\n")
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Warning: failed to scan content for comment removal: %v\n", err)
		return content
	}

	return result.String()
}

// 修改：返回当前文件的带索引内容块（而非直接写入extraMeta）
func processGenCodeFile(outPutFilePath string, filePath string, fieldMetas *[]meta.FieldMeta, extraMeta *meta.FileExtraMeta) ([]meta.IndexedContentBlock, error) {
	relativePath, err := filepath.Rel(outPutFilePath, filePath)
	if err != nil {
		fmt.Printf("Failed to get relative path for %s: %v\n", filePath, err)
		return nil, err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %v", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file failed: %v", err)
	}

	if len(lines) == 0 || lines[0] != "//@genCode" {
		return nil, nil
	}

	var inContentBlock bool
	var currentContent string
	var currentContentIndex int
	var fileContentBlocks []meta.IndexedContentBlock // 当前文件的带索引内容块

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 解析//@include注解
		if matches := includeAnnotationRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			incPath := strings.TrimSpace(matches[1])
			if incPath != "" {
				extraMeta.IncludePaths = append(extraMeta.IncludePaths, incPath)
			}
			continue
		}

		// 解析//@content(index)开始标记
		if matches := contentStartRegex.FindStringSubmatch(trimmedLine); len(matches) >= 2 {
			indexStr := matches[1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				fmt.Printf("Invalid content index '%s' in file %s: %v\n", indexStr, filePath, err)
				continue
			}
			inContentBlock = true
			currentContent = ""
			currentContentIndex = index
			continue
		}

		// 解析//@endContent结束标记
		if contentEndRegex.MatchString(trimmedLine) {
			inContentBlock = false
			trimmedContent := strings.TrimSpace(currentContent)
			if trimmedContent != "" {
				cleanContent := removeLineComments(currentContent)
				fileContentBlocks = append(fileContentBlocks, meta.IndexedContentBlock{
					Index:   currentContentIndex,
					Content: cleanContent,
				})
			}
			continue
		}

		// 收集content块内的内容
		if inContentBlock {
			currentContent += line + "\n"
		}
	}

	// 解析类和字段逻辑（无修改）
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

	// 返回当前文件的带索引内容块
	return fileContentBlocks, nil
}

// generateCPPHeaderFile 生成C++头文件（内容块已全局排序）
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

	var bodyContent strings.Builder
	classFields := make(map[string][]meta.FieldMeta)
	for _, fm := range fieldMetas {
		if fm.ClassName == "" {
			continue
		}
		classFields[fm.ClassName] = append(classFields[fm.ClassName], fm)
	}

	depsMap := extractClassDependencies(classFields)
	sortedClasses := topologicalSort(depsMap)
	inheritanceMap := buildClassInheritance(classFields, fieldMetas)

	bodyContent.WriteString("namespace toml {\n\n")
	// 注入全局排序后的内容块
	if len(contentBlocks) > 0 {
		bodyContent.WriteString("// Injected by //@content annotations (globally sorted by index)\n")
		for _, content := range contentBlocks {
			bodyContent.WriteString(content)
			bodyContent.WriteString("\n")
		}
		bodyContent.WriteString("\n")
	}

	for _, className := range sortedClasses {
		allFields := collectAllFields(className, classFields, inheritanceMap)
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

		for _, fm := range allFields {
			snakeName := toSnakeCase(fm.Name)
			bodyContent.WriteString("            r.")
			bodyContent.WriteString(fm.Name)
			if fm.Default == "" {
				bodyContent.WriteString(" = toml::find<")
				bodyContent.WriteString(fm.Type)
				bodyContent.WriteString(">")
				bodyContent.WriteString("(v, \"")
				bodyContent.WriteString(snakeName) // TOML key 使用蛇形
				bodyContent.WriteString("\");\n")
			} else {
				bodyContent.WriteString(" = toml::find_or<")
				bodyContent.WriteString(fm.Type)
				bodyContent.WriteString(">")
				bodyContent.WriteString("(v, \"")
				bodyContent.WriteString(snakeName) // TOML key 使用蛇形
				bodyContent.WriteString("\",")
				if fm.Type == "std::string" {
					bodyContent.WriteString("\"")
				}
				bodyContent.WriteString(fm.Default)
				if fm.Type == "std::string" {
					bodyContent.WriteString("\");")
				} else {
					bodyContent.WriteString(");")
				}
				bodyContent.WriteString("\n")
			}
		}

		bodyContent.WriteString("            return r;\n")
		bodyContent.WriteString("        }\n")
		bodyContent.WriteString("    };\n\n")
	}

	bodyContent.WriteString("}\n")
	headerContent.WriteString(bodyContent.String())

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

// buildClassInheritance 构建类的继承层级
func buildClassInheritance(classFields map[string][]meta.FieldMeta, fieldMetas []meta.FieldMeta) map[string]string {
	inheritanceMap := make(map[string]string)
	classParent := make(map[string]string)
	for _, fm := range fieldMetas {
		if fm.ClassName == "" || fm.ParentClassName == "" {
			continue
		}
		parentName := fm.ParentClassName
		if !strings.Contains(parentName, "::") && strings.Contains(fm.ClassName, "::") {
			ns := strings.Split(fm.ClassName, "::")[0] + "::"
			parentName = ns + parentName
		}
		classParent[fm.ClassName] = parentName
	}
	for className, parent := range classParent {
		if _, exists := classFields[parent]; exists {
			inheritanceMap[className] = parent
		}
	}
	return inheritanceMap
}

// collectAllFields 递归收集类的所有字段
func collectAllFields(className string, classFields map[string][]meta.FieldMeta, inheritanceMap map[string]string) []meta.FieldMeta {
	var allFields []meta.FieldMeta
	if parent, exists := inheritanceMap[className]; exists {
		parentFields := collectAllFields(parent, classFields, inheritanceMap)
		allFields = append(allFields, parentFields...)
	}

	fieldNameSet := make(map[string]bool)
	for _, f := range allFields {
		fieldNameSet[f.Name] = true
	}
	currentFields := classFields[className]
	for _, f := range currentFields {
		if !fieldNameSet[f.Name] {
			allFields = append(allFields, f)
			fieldNameSet[f.Name] = true
		}
	}
	return allFields
}

// generateJSONFile 生成JSON元信息文件
func generateJSONFile(outputPath string, fieldMetas []meta.FieldMeta) error {
	data, err := json.MarshalIndent(fieldMetas, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}
	file, err := os.Create(filepath.Join(outputPath, "field_meta.gen.json"))
	if err != nil {
		return fmt.Errorf("failed to create json file: %v", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write json file: %v", err)
	}

	fmt.Printf("Successfully generated JSON file: %s\n", filepath.Join(outputPath, "field_meta.gen.json"))
	return nil
}

// extractClassDependencies 提取类依赖关系
func extractClassDependencies(classFields map[string][]meta.FieldMeta) map[string]meta.ClassDependency {
	depsMap := make(map[string]meta.ClassDependency)
	basicTypeRegex := regexp.MustCompile(`^(bool|int|uint8_t|uint32_t|uint64_t|float|size_t|std::string|std::vector<.+>)$`)
	vectorRegex := regexp.MustCompile(`^std::vector<(.+)>$`)
	allClasses := make(map[string]bool)
	for className := range classFields {
		allClasses[className] = true
	}

	for className, fields := range classFields {
		deps := make(map[string]bool)
		for _, fm := range fields {
			fieldType := fm.Type
			if basicTypeRegex.MatchString(fieldType) {
				continue
			}
			if matches := vectorRegex.FindStringSubmatch(fieldType); len(matches) >= 2 {
				fieldType = matches[1]
			}
			if allClasses[fieldType] {
				deps[fieldType] = true
			}
		}
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

// topologicalSort 拓扑排序类
func topologicalSort(depsMap map[string]meta.ClassDependency) []string {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for className := range depsMap {
		inDegree[className] = 0
		adj[className] = []string{}
	}

	for className, dep := range depsMap {
		for _, d := range dep.Deps {
			adj[d] = append(adj[d], className)
			inDegree[className]++
		}
	}

	var queue []string
	for className, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, className)
		}
	}

	var sortedClasses []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sortedClasses = append(sortedClasses, current)
		for _, neighbor := range adj[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sortedClasses) != len(depsMap) {
		sortedClasses = make([]string, 0, len(depsMap))
		for className := range depsMap {
			sortedClasses = append(sortedClasses, className)
		}
		sort.Strings(sortedClasses)
	}
	return sortedClasses
}

// genCodeCmd 主命令
var genCodeCmd = &cobra.Command{
	Use:   "genCode",
	Short: "Parse C++ files with //@genCode annotation and generate FieldMeta",
	Long:  `Parse all .cpp/.h files with //@genCode annotation, extract class/struct field info and output FieldMeta/C++ Header/JSON files.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Printf("Failed to get current directory: %v\n", err)
			return
		}
		outputPath, _ := cmd.Flags().GetString("outputPath")
		outputType, _ := cmd.Flags().GetInt8("outputType")
		if outputPath == "" {
			outputPath = dir
		}
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				fmt.Printf("Failed to create output directory: %v\n", err)
				return
			}
		}

		var fieldMetas []meta.FieldMeta
		var allIncludePaths []string
		var allIndexedContentBlocks []meta.IndexedContentBlock // 全局带索引内容块

		// 遍历目录处理文件
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("access file failed: %v\n", err)
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".h") && !strings.HasSuffix(path, ".cpp") {
				return nil
			}

			var extraMeta meta.FileExtraMeta
			// 处理文件，获取当前文件的带索引内容块
			fileContentBlocks, err := processGenCodeFile(outputPath, path, &fieldMetas, &extraMeta)
			if err != nil {
				fmt.Printf("process file %s failed: %v\n", path, err)
				return err
			}

			// 收集include路径（去重）
			includeSet := make(map[string]struct{})
			for _, path := range extraMeta.IncludePaths {
				if path != "" {
					includeSet[path] = struct{}{}
				}
			}
			for path := range includeSet {
				allIncludePaths = append(allIncludePaths, path)
			}

			// 收集当前文件的带索引内容块到全局列表
			allIndexedContentBlocks = append(allIndexedContentBlocks, fileContentBlocks...)

			return nil
		})

		if err != nil {
			fmt.Printf("walk dir failed: %v\n", err)
			return
		}

		// 关键修复：全局按索引排序所有内容块
		sort.Slice(allIndexedContentBlocks, func(i, j int) bool {
			return allIndexedContentBlocks[i].Index < allIndexedContentBlocks[j].Index
		})

		// 修复：转换为字符串切片（去重） - 修正map使用错误
		var allContentBlocks []string
		contentSet := make(map[string]bool) // 改为bool类型的map
		for _, block := range allIndexedContentBlocks {
			trimmed := strings.TrimSpace(block.Content)
			if trimmed != "" && !contentSet[trimmed] { // 正确判断是否存在
				allContentBlocks = append(allContentBlocks, block.Content)
				contentSet[trimmed] = true // 正确赋值bool类型
			}
		}

		// 输出FieldMeta信息
		fmt.Println("=== All FieldMeta Information ===")
		if len(fieldMetas) == 0 {
			fmt.Println("No FieldMeta found.")
		} else {
			fmt.Printf("%-4s %-30s %-25s %-20s %-30s %-20s %-20s %s\n",
				"NO", "RelativePath", "Class", "Parent Class", "Type", "Name(snake)", "Default", "Note")
			fmt.Println(strings.Repeat("-", 180))
			for i, fm := range fieldMetas {
				parent := fm.ParentClassName
				if parent == "" {
					parent = "-"
				}
				snakeName := toSnakeCase(fm.Name)
				fmt.Printf("%-4d %-30s %-25s %-20s %-30s %-20s %-20s %s\n",
					i+1, fm.RelativePath, fm.ClassName, parent, fm.Type, snakeName, fm.Default, fm.Note)
			}
		}

		// 生成文件
		switch outputType {
		case 1:
			if err := generateCPPHeaderFile(outputPath, fieldMetas, allIncludePaths, allContentBlocks); err != nil {
				fmt.Printf("generate header failed: %v\n", err)
			}
		case 2:
			if err := generateJSONFile(outputPath, fieldMetas); err != nil {
				fmt.Printf("Failed to generate JSON file: %v\n", err)
			}
		case 0:
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
	genCodeCmd.Flags().StringP("outputPath", "o", "", "File output path (default: current directory)")
	genCodeCmd.Flags().Int8P("outputType", "t", 0, "Output type (0=none, 1=CPP header, 2=JSON meta info)")
}
