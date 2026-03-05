/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"GlimmerWorksCli/meta"
	"bufio"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

	// 第四步：遍历所有//@genNextLine标记，解析字段
	for lineIdx, line := range lines {
		// 匹配标记行
		if !strings.HasPrefix(line, "//@genNextLine(") {
			continue
		}

		// 标记行的下一行索引
		fieldLineIdx := lineIdx + 1
		if fieldLineIdx >= len(lines) {
			fmt.Printf("Line %d: Found genNextLine mark but reached end of file\n", lineIdx+1)
			continue
		}
		fieldLine := lines[fieldLineIdx]
		fmt.Printf("Line %d: Found genNextLine mark, field line: %s\n", lineIdx+1, fieldLine)

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

		// 添加到FieldMeta
		*fieldMetas = append(*fieldMetas, meta.FieldMeta{
			ClassName:       currentClassName,
			ParentClassName: currentParentName,
			Type:            fieldType,
			Name:            fieldName,
			Default:         fieldDefault,
		})
	}

	fmt.Println("=== Processing completed ===\n")
}

// genCodeCmd 保持不变，仅优化输出格式
var genCodeCmd = &cobra.Command{
	Use:   "genCode",
	Short: "Parse C++ files with //@genCode annotation and generate FieldMeta",
	Long:  `Parse all .cpp/.h files with //@genCode annotation, extract class/struct field info (type/name/default) and output FieldMeta.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Printf("Failed to get current directory: %v\n", err)
			return
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

		// 输出优化：对齐显示
		fmt.Println("=== All FieldMeta Information ===")
		if len(fieldMetas) == 0 {
			fmt.Println("No FieldMeta found.")
		} else {
			// 表头
			fmt.Printf("%-4s %-25s %-20s %-30s %-20s %s\n",
				"NO", "Class", "Parent Class", "Type", "Name", "Default")
			fmt.Println(strings.Repeat("-", 120))
			// 内容
			for i, fm := range fieldMetas {
				parent := fm.ParentClassName
				if parent == "" {
					parent = "-"
				}
				fmt.Printf("%-4d %-25s %-20s %-30s %-20s %s\n",
					i+1, fm.ClassName, parent, fm.Type, fm.Name, fm.Default)
			}
		}

		if err != nil {
			fmt.Printf("Failed to traverse directory: %v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(genCodeCmd)
}
