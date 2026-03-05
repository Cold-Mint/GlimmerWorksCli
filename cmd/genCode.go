/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"GlimmerWorksCli/meta"
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// 正则表达式预编译（全面升级）
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
)

// C++ toml反序列化模板
const cppTemplateStr = `#include <toml++/toml.h>
#include "glimmer/mod/Resource.h"
#include "glimmer/mod/ResourceRef.h"

using namespace toml;

{{range $class, $fields := .ClassFields}}
template<>
struct from<glimmer::{{$class}}> {
    static glimmer::{{$class}} from_toml(const value &v) {
        glimmer::{{$class}} obj;
        {{/* 先处理父类字段 */}}
        {{if $parent := index $.ClassParents $class}}
        // Inherit from glimmer::{{$parent}}
        auto parentObj = from<glimmer::{{$parent}}>::from_toml(v);
        obj = static_cast<glimmer::{{$class}}>(parentObj);
        {{end}}
        
        {{/* 处理当前类字段 */}}
        {{range $fields}}
        {{if strings.Contains .Type "std::vector"}}
        // Vector field: {{.Name}}
        auto {{.Name}}_val = find_or_default<{{.Type}}>(v, "{{.Name}}");
        for (auto &item : {{.Name}}_val) {
            obj.Add{{strings.Title .Name}}(item);
        }
        {{else if eq .Type "bool"}}
        // Bool field: {{.Name}}
        obj.Set{{strings.Title .Name}}(find_or_default<bool>(v, "{{.Name}}", {{.Default}}));
        {{else if eq .Type "int" || eq .Type "uint32_t" || eq .Type "uint64_t" || eq .Type "float" || eq .Type "uint8_t"}}
        // Numeric field: {{.Name}}
        obj.Set{{strings.Title .Name}}(find_or_default<{{.Type}}>(v, "{{.Name}}", {{.Default}}));
        {{else if eq .Type "std::string"}}
        // String field: {{.Name}}
        obj.Set{{strings.Title .Name}}(find_or_default<std::string>(v, "{{.Name}}", "{{.Default}}"));
        {{else}}
        // Custom type field: {{.Name}}
        obj.Set{{strings.Title .Name}}(from<{{.Type}}>::from_toml(find(v, "{{.Name}}")));
        {{end}}
        {{end}}
        
        return obj;
    }
};
{{end}}
`

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
func processGenCodeFile(filePath string, fieldMetas *[]meta.FieldMeta) {
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

		// 提取字段信息
		fieldType := strings.TrimSpace(fieldMatches[1])
		fieldName := strings.TrimSpace(fieldMatches[2])
		fieldDefault := strings.TrimSpace(fieldMatches[4])

		// 处理默认值格式（字符串加引号，布尔值小写，空值处理）
		switch {
		case fieldDefault == "":
			// 根据类型设置默认值
			switch fieldType {
			case "std::string":
				fieldDefault = `""`
			case "bool":
				fieldDefault = "false"
			case "int", "uint32_t", "uint64_t", "float", "uint8_t":
				fieldDefault = "0"
			default:
				fieldDefault = ""
			}
		case fieldType == "std::string" && !strings.HasPrefix(fieldDefault, "\""):
			fieldDefault = fmt.Sprintf(`"%s"`, fieldDefault)
		case fieldType == "bool":
			fieldDefault = strings.ToLower(fieldDefault)
		}

		// 添加到FieldMeta（新增Note字段）
		*fieldMetas = append(*fieldMetas, meta.FieldMeta{
			ClassName:       currentClassName,
			ParentClassName: currentParentName,
			Type:            fieldType,
			Name:            fieldName,
			Default:         fieldDefault,
			Note:            note, // 填充注解内容
		})
	}

	fmt.Println("=== Processing completed ===\n")
}

// generateCPPFile 生成C++ toml反序列化代码文件
func generateCPPFile(outputPath string, fieldMetas []meta.FieldMeta) error {
	// 整理类字段和父类映射
	classFields := make(map[string][]meta.FieldMeta) // 类名 -> 字段列表
	classParents := make(map[string]string)          // 类名 -> 父类名
	seenClasses := make(map[string]bool)

	// 分组字段并记录父类
	for _, fm := range fieldMetas {
		if fm.ClassName == "" {
			continue
		}
		classFields[fm.ClassName] = append(classFields[fm.ClassName], fm)
		if fm.ParentClassName != "" && !seenClasses[fm.ClassName] {
			classParents[fm.ClassName] = fm.ParentClassName
			seenClasses[fm.ClassName] = true
		}
	}

	// 创建模板函数映射
	funcMap := template.FuncMap{
		"strings.Contains": strings.Contains,
		"strings.Title":    strings.Title,
		"eq":               eq,
	}

	// 解析模板
	tpl, err := template.New("cpp_toml").Funcs(funcMap).Parse(cppTemplateStr)
	if err != nil {
		return fmt.Errorf("failed to parse cpp template: %v", err)
	}

	// 创建输出文件
	file, err := os.Create(filepath.Join(outputPath, "toml_deserializer.gen.cpp"))
	if err != nil {
		return fmt.Errorf("failed to create cpp file: %v", err)
	}
	defer file.Close()

	// 渲染模板
	data := map[string]interface{}{
		"ClassFields":  classFields,
		"ClassParents": classParents,
	}
	if err := tpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to render cpp template: %v", err)
	}

	fmt.Printf("Successfully generated C++ file: %s\n", filepath.Join(outputPath, "toml_deserializer.gen.cpp"))
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

// eq 模板函数：相等判断（兼容template包）
func eq(a, b interface{}) bool {
	return a == b
}

// genCodeCmd 优化输出格式，新增文件生成功能
var genCodeCmd = &cobra.Command{
	Use:   "genCode",
	Short: "Parse C++ files with //@genCode annotation and generate FieldMeta",
	Long:  `Parse all .cpp/.h files with //@genCode annotation, extract class/struct field info (type/name/default/note) and output FieldMeta/C++/JSON files.`,
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

			processGenCodeFile(path, &fieldMetas)
			return nil
		})

		// 输出所有FieldMeta信息
		fmt.Println("=== All FieldMeta Information ===")
		if len(fieldMetas) == 0 {
			fmt.Println("No FieldMeta found.")
		} else {
			// 表头（调整列宽，新增Note列）
			fmt.Printf("%-4s %-25s %-20s %-30s %-20s %-20s %s\n",
				"NO", "Class", "Parent Class", "Type", "Name", "Default", "Note")
			fmt.Println(strings.Repeat("-", 150))
			// 内容
			for i, fm := range fieldMetas {
				parent := fm.ParentClassName
				if parent == "" {
					parent = "-"
				}
				fmt.Printf("%-4d %-25s %-20s %-30s %-20s %-20s %s\n",
					i+1, fm.ClassName, parent, fm.Type, fm.Name, fm.Default, fm.Note)
			}
		}

		// 根据outputType生成文件
		switch outputType {
		case 1:
			// 生成C++文件
			if err := generateCPPFile(outputPath, fieldMetas); err != nil {
				fmt.Printf("Failed to generate C++ file: %v\n", err)
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
			fmt.Printf("Invalid outputType: %d (0=none, 1=cpp, 2=json)\n", outputType)
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
	// 0为不输出，1为输出cpp文件，2为输出json文件。默认0
	genCodeCmd.Flags().Int8P("outputType", "t", 0, "Output type (0=none, 1=CPP toml deserializer, 2=JSON meta info)")
}
