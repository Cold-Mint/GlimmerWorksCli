/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// FoundLib 存储库文件路径信息
type FoundLib struct {
	SymLinkPath string // 软链接绝对路径
	RealPath    string // 真实文件绝对路径
	SymRelPath  string // 软链接相对路径
	RealRelPath string // 真实文件相对路径
	IsSymlink   bool   // 是否为软链接
}

// 正则匹配库文件版本号 (例如 .so.1.2.3)
var versionRegex = regexp.MustCompile(`(\.\d+)+$`)

// 正则匹配库后缀 (.so, .dll, .dylib)
var suffixRegex = regexp.MustCompile(`\.(so|dll|dylib)`)

// dep2cmakeCmd represents the dep2cmake command
var dep2cmakeCmd = &cobra.Command{
	Use:   "dep2cmake",
	Short: "Find missing library paths via ldd and output CMake install code",
	Long:  "Analyze executable dependencies with ldd, find missing libraries, resolve symlinks and generate CMake install commands",
	Run: func(cmd *cobra.Command, args []string) {
		// 获取当前工作目录（用于计算相对路径）
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error: Failed to get current directory: %v\n", err)
			return
		}

		// 1. 校验命令行参数
		buildPath, err := cmd.Flags().GetString("buildPath")
		if err != nil || buildPath == "" {
			fmt.Println("Error: --buildPath parameter must be specified")
			return
		}

		execFile, err := cmd.Flags().GetString("executableFile")
		if err != nil || execFile == "" {
			fmt.Println("Error: --executableFile parameter must be specified")
			return
		}

		// 校验可执行文件
		execInfo, err := os.Stat(execFile)
		if err != nil {
			fmt.Printf("Error: Executable file does not exist: %v\n", err)
			return
		}
		if execInfo.IsDir() {
			fmt.Println("Error: executableFile must be a file, not a directory")
			return
		}

		// 校验构建目录
		buildInfo, err := os.Stat(buildPath)
		if err != nil {
			fmt.Printf("Error: Build directory does not exist: %v\n", err)
			return
		}
		if !buildInfo.IsDir() {
			fmt.Println("Error: buildPath must be a directory")
			return
		}

		// 2. 执行ldd获取缺失依赖库
		missingLibs, err := getMissingDependencies(execFile)
		if err != nil {
			fmt.Printf("Error: Failed to run ldd and parse dependencies: %v\n", err)
			return
		}
		totalMissing := len(missingLibs)
		if totalMissing == 0 {
			fmt.Println("✅ No missing dependent libraries detected")
			return
		}
		fmt.Printf("🔍 Detected %d missing dependent libraries\n", totalMissing)

		// 3. 遍历目录查找库文件
		var foundLibs []FoundLib
		err = filepath.WalkDir(buildPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				fmt.Printf("Warning: Failed to traverse directory %s: %v\n", path, walkErr)
				return walkErr
			}
			if d.IsDir() {
				return nil
			}

			// 匹配缺失的库文件
			fileName := d.Name()
			if _, exists := missingLibs[fileName]; !exists {
				return nil
			}

			// 核心：严格区分软链接和真实文件路径
			var symLinkAbs, realAbs string
			isSymlink := false

			// 判断是否为软链接
			if d.Type()&os.ModeSymlink != 0 {
				isSymlink = true
				symLinkAbs, _ = filepath.Abs(path)
				// 解析软链接指向的真实文件
				realPath, err := filepath.EvalSymlinks(path)
				if err != nil {
					realAbs = symLinkAbs
				} else {
					realAbs, _ = filepath.Abs(realPath)
				}
			} else {
				// 普通文件
				isSymlink = false
				symLinkAbs, _ = filepath.Abs(path)
				realAbs = symLinkAbs
			}

			// 计算相对路径
			symRel, _ := filepath.Rel(currentDir, symLinkAbs)
			realRel, _ := filepath.Rel(currentDir, realAbs)

			// 添加到结果集
			foundLibs = append(foundLibs, FoundLib{
				SymLinkPath: symLinkAbs,
				RealPath:    realAbs,
				SymRelPath:  symRel,
				RealRelPath: realRel,
				IsSymlink:   isSymlink,
			})

			return nil
		})

		if err != nil {
			fmt.Printf("Error: Failed to traverse build directory: %v\n", err)
			return
		}

		// 统计数据
		foundCount := len(foundLibs)
		stillMissingCount := totalMissing - foundCount

		// 输出库路径结果
		fmt.Println("\n=====================================")
		fmt.Println("📦 Found Missing Dependent Libraries")
		fmt.Println("=====================================")
		if foundCount == 0 {
			fmt.Println("❌ No missing dependent libraries found in the build directory")
		} else {
			for _, lib := range foundLibs {
				if !lib.IsSymlink {
					fmt.Printf("Library: %s\n", lib.RealRelPath)
				} else {
					fmt.Printf("Symlink: %s\nReal File: %s\n", lib.SymRelPath, lib.RealRelPath)
				}
				fmt.Println("-------------------------------------")
			}
		}

		// 输出统计
		fmt.Println("\n=====================================")
		fmt.Println("📊 Statistics Summary")
		fmt.Println("=====================================")
		fmt.Printf("Total missing libraries:  %d\n", totalMissing)
		fmt.Printf("Libraries found:          %d\n", foundCount)
		fmt.Printf("Libraries still missing:  %d\n", stillMissingCount)
		fmt.Println("=====================================\n")

		// ===================== 生成标准CMake Install代码 =====================
		if foundCount > 0 {
			generateCMakeInstallCode(foundLibs)
		}
	},
}

// generateCMakeInstallCode 生成标准CMake install(FILES) 代码
func generateCMakeInstallCode(libs []FoundLib) {
	fmt.Println("=====================================")
	fmt.Println("🔧 AUTO-GENERATED CMAKE INSTALL CODE")
	fmt.Println("=====================================")
	fmt.Println("# ==== CROSS-PLATFORM LIBRARY SUFFIX ====")
	fmt.Println("if(WIN32)")
	fmt.Println("    set(LIB_SUFFIX dll)")
	fmt.Println("elseif(APPLE)")
	fmt.Println("    set(LIB_SUFFIX dylib)")
	fmt.Println("else()")
	fmt.Println("    set(LIB_SUFFIX so)")
	fmt.Println("endif()")
	fmt.Println()
	fmt.Println("# ==== AUTO-GENERATED INSTALL COMMANDS ====")
	fmt.Println()

	for _, lib := range libs {
		if lib.IsSymlink {
			// 软链接：安装 软链接 + 真实文件
			generateInstallCmd(lib.SymRelPath, lib.RealRelPath, true)
		} else {
			// 普通文件
			generateInstallCmd(lib.RealRelPath, "", false)
		}
		fmt.Println()
	}
}

// generateInstallCmd 生成单条/两条 install(FILES) 命令
func generateInstallCmd(relPath string, realRelPath string, isSymlink bool) {
	// 路径转换 + 替换库后缀
	cmakePath := convertToCMakePath(relPath)
	cmakePath = suffixRegex.ReplaceAllString(cmakePath, ".${LIB_SUFFIX}")
	fullPath := cmakePath
	basePath := removeVersionSuffix(cmakePath)

	// 软链接处理第二个文件
	var realFullPath string
	if isSymlink {
		realCmakePath := convertToCMakePath(realRelPath)
		realCmakePath = suffixRegex.ReplaceAllString(realCmakePath, ".${LIB_SUFFIX}")
		realFullPath = realCmakePath
	}

	// 生成标准 CMake install 命令
	fmt.Println("if (WIN32)")
	fmt.Printf("    install(FILES \"%s\" DESTINATION ${LIB_DIR})\n", basePath)
	fmt.Println("else ()")
	if isSymlink {
		// 软链接：安装两个文件
		fmt.Printf("    install(FILES \"%s\" DESTINATION ${LIB_DIR})\n", fullPath)
		fmt.Printf("    install(FILES \"%s\" DESTINATION ${LIB_DIR})\n", realFullPath)
	} else {
		// 普通文件：安装带版本的库
		fmt.Printf("    install(FILES \"%s\" DESTINATION ${LIB_DIR})\n", fullPath)
	}
	fmt.Println("endif ()")
}

// convertToCMakePath 路径转换：第一个目录替换为 ${CMAKE_BINARY_DIR}
func convertToCMakePath(relPath string) string {
	idx := strings.Index(relPath, "/")
	if idx == -1 {
		return "${CMAKE_BINARY_DIR}/" + relPath
	}
	return "${CMAKE_BINARY_DIR}/" + relPath[idx+1:]
}

// removeVersionSuffix 移除版本号后缀
func removeVersionSuffix(path string) string {
	return versionRegex.ReplaceAllString(path, "")
}

// getMissingDependencies 执行ldd解析缺失依赖
func getMissingDependencies(execFile string) (map[string]struct{}, error) {
	cmd := exec.Command("ldd", execFile)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	missing := make(map[string]struct{})
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "not found") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				libName := fields[0]
				missing[libName] = struct{}{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return missing, nil
}

func init() {
	rootCmd.AddCommand(dep2cmakeCmd)
	dep2cmakeCmd.PersistentFlags().StringP("buildPath", "b", "", "Build output root directory (required)")
	dep2cmakeCmd.PersistentFlags().StringP("executableFile", "e", "", "Path to executable file (required)")
	_ = dep2cmakeCmd.MarkPersistentFlagRequired("buildPath")
	_ = dep2cmakeCmd.MarkPersistentFlagRequired("executableFile")
}
