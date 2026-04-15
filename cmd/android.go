/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// 安卓项目固定目录结构（标准Android Studio项目）
const (
	androidJniLibsPath = "app/src/main/jniLibs" // so库目标目录
	androidAssetsPath  = "app/src/main/assets"  // 资源文件目标目录
	androidLibsPath    = "app/libs"             // aar包目标目录（标准）
	indexFileName      = "index.json"           // 资源索引文件(JSON)
)

// 资源索引结构体
type IndexEntry struct {
	IsFile bool   `json:"is_file"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

var androidCmd = &cobra.Command{
	Use:   "android",
	Short: "Install/Uninstall GlimmerWorks components to Android project",
	Long: `One-click install GlimmerWorks SO library, SDL3 SO, resource files and SDL AAR to Android project, or uninstall and clean all installed files.
Support specifying operation type, Android project root directory, CMake build directory (required only for installation)`,
	Run: runAndroidCommand,
}

// 命令行参数
var (
	action      string
	androidRoot string
	buildDir    string
	arch        string
	skipHidden  bool
	sdlAarPath  string
	// 存储扫描到的所有SO库（全路径）
	allSOFiles []string
)

func init() {
	rootCmd.AddCommand(androidCmd)

	androidCmd.Flags().StringVarP(&action, "action", "a", "", "Required: Operation type [install|uninstall]")
	androidCmd.Flags().StringVarP(&androidRoot, "android-root", "r", "", "Required: Android project root directory")
	androidCmd.Flags().StringVarP(&buildDir, "build-dir", "b", "", "Required for install: CMake build directory")
	androidCmd.Flags().StringVarP(&arch, "arch", "s", "", "Required for install: Specify SO architecture (arm64-v8a/armeabi-v7a/x86/x86_64)")
	androidCmd.Flags().StringVarP(&sdlAarPath, "sdl-aar", "l", "", "Required for install: Path to SDL AAR package file")
	androidCmd.Flags().BoolVarP(&skipHidden, "skip-hidden", "k", true, "Skip hidden folders starting with . (default: true)")

	_ = androidCmd.MarkFlagRequired("action")
	_ = androidCmd.MarkFlagRequired("android-root")
}

func runAndroidCommand(cmd *cobra.Command, args []string) {
	action = strings.ToLower(action)
	if action != "install" && action != "uninstall" {
		fmt.Printf("❌ Error: action parameter can only be install or uninstall\n")
		os.Exit(1)
	}

	if !isDirExists(androidRoot) {
		fmt.Printf("❌ Error: Android project directory does not exist -> %s\n", androidRoot)
		os.Exit(1)
	}

	switch action {
	case "install":
		if buildDir == "" || !isDirExists(buildDir) {
			fmt.Printf("❌ Error: A valid CMake build directory must be specified during installation\n")
			os.Exit(1)
		}
		if arch == "" {
			fmt.Printf("❌ Error: SO architecture (--arch/-s) must be specified during installation\n")
			os.Exit(1)
		}
		if sdlAarPath == "" || !isFileExists(sdlAarPath) {
			fmt.Printf("❌ Error: Valid SDL AAR file path (--sdl-aar/-l) must be specified during installation\n")
			os.Exit(1)
		}
		// 🔥 核心修复：递归扫描整个build目录+所有子目录的.so文件
		allSOFiles = scanAllSOFilesRecursive()
		if len(allSOFiles) == 0 {
			fmt.Println("❌ Error: No .so files found in build directory and subdirectories")
			os.Exit(1)
		}
		if err := installToAndroid(); err != nil {
			fmt.Printf("❌ Installation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Installation completed successfully!")
	case "uninstall":
		if err := uninstallFromAndroid(); err != nil {
			fmt.Printf("❌ Uninstallation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Uninstallation completed successfully!")
	}
}

// ------------------------------
// 安装逻辑
// ------------------------------
func installToAndroid() error {
	assetsDir := filepath.Join(androidRoot, androidAssetsPath)
	jniLibsDir := filepath.Join(androidRoot, androidJniLibsPath)
	libsDir := filepath.Join(androidRoot, androidLibsPath)

	// 创建目录
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}
	if err := os.MkdirAll(jniLibsDir, 0755); err != nil {
		return fmt.Errorf("failed to create jniLibs directory: %w", err)
	}
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		return fmt.Errorf("failed to create libs directory: %w", err)
	}

	// 1. 批量拷贝所有扫描到的SO库
	fmt.Println("🔍 Copying all SO libraries from build directory and subdirectories...")
	if err := copyAllSOFiles(jniLibsDir); err != nil {
		return err
	}

	// 2. 生成并打印Java库名代码
	fmt.Println("\n📝 Generated Java Library Names Code:")
	javaCode := generateJavaLibraryCode(allSOFiles)
	fmt.Println(javaCode)
	fmt.Println()

	// 3. 拷贝SDL AAR
	fmt.Println("📦 Copying SDL AAR package...")
	aarFileName := filepath.Base(sdlAarPath)
	targetAarPath := filepath.Join(libsDir, aarFileName)
	if err := copyFile(sdlAarPath, targetAarPath); err != nil {
		return fmt.Errorf("failed to copy SDL AAR: %w", err)
	}

	// 4. 拷贝资源文件
	fmt.Println("📂 Copying resource files...")
	resourceFiles := []string{"LICENSE", "config.toml"}
	resourceDirs := []string{"mods", "langs"}

	for _, file := range resourceFiles {
		src := filepath.Join(buildDir, file)
		if !isFileExists(src) {
			return fmt.Errorf("resource file does not exist: %s", src)
		}
		dst := filepath.Join(assetsDir, file)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy file %s: %w", file, err)
		}
	}

	for _, dir := range resourceDirs {
		src := filepath.Join(buildDir, dir)
		if !isDirExists(src) {
			return fmt.Errorf("resource directory does not exist: %s", src)
		}
		dst := filepath.Join(assetsDir, dir)
		if err := copyDir(src, dst); err != nil {
			return fmt.Errorf("failed to copy directory %s: %w", dir, err)
		}
	}

	// 5. 生成资源索引JSON
	fmt.Println("📝 Generating assets index file...")
	indexPath := filepath.Join(assetsDir, indexFileName)
	if err := generateIndexJson(assetsDir, indexPath); err != nil {
		return fmt.Errorf("failed to generate %s: %w", indexFileName, err)
	}

	return nil
}

// 🔥 核心修复：递归扫描整个build目录 + 所有子目录，查找全部.so文件
func scanAllSOFilesRecursive() []string {
	var soFiles []string

	err := filepath.Walk(buildDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 跳过隐藏文件夹（遵循skipHidden参数）
		if skipHidden && f.IsDir() && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		// 匹配所有 .so 后缀的文件
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".so") {
			soFiles = append(soFiles, path)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("⚠️ Warning: error scanning SO files: %v\n", err)
	}

	return soFiles
}

// 批量拷贝所有SO库到指定架构目录
func copyAllSOFiles(targetJniLibsDir string) error {
	targetArchDir := filepath.Join(targetJniLibsDir, arch)
	if err := os.MkdirAll(targetArchDir, 0755); err != nil {
		return fmt.Errorf("failed to create architecture directory %s: %w", arch, err)
	}

	for _, soPath := range allSOFiles {
		soName := filepath.Base(soPath)
		targetSoPath := filepath.Join(targetArchDir, soName)
		if err := copyFile(soPath, targetSoPath); err != nil {
			return fmt.Errorf("failed to copy %s: %w", soName, err)
		}
		fmt.Printf("✅ Copied: %s\n", soName)
	}
	return nil
}

// 生成指定格式的Java代码
func generateJavaLibraryCode(soFiles []string) string {
	var libNames []string
	for _, soPath := range soFiles {
		soName := filepath.Base(soPath)
		// 提取库名：libXXX.so → XXX
		name := strings.TrimPrefix(soName, "lib")
		name = strings.TrimSuffix(name, ".so")
		libNames = append(libNames, fmt.Sprintf(`"%s"`, name))
	}

	// 拼接Java代码格式（完全匹配你要求的格式）
	code := "    return new String[]{\n"
	for i, name := range libNames {
		if i == len(libNames)-1 {
			code += fmt.Sprintf("        %s\n", name)
		} else {
			code += fmt.Sprintf("        %s,\n", name)
		}
	}
	code += "    };"
	return code
}

// 生成资源索引JSON
func generateIndexJson(assetsDir, indexPath string) error {
	var entries []IndexEntry
	err := filepath.Walk(assetsDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == indexPath {
			return nil
		}
		if skipHidden && f.IsDir() && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}

		relPath, _ := filepath.Rel(assetsDir, path)
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		sha256Str := ""
		if !f.IsDir() {
			sha256Str, _ = computeFileSHA256(path)
		}

		entries = append(entries, IndexEntry{
			IsFile: !f.IsDir(),
			Path:   relPath,
			SHA256: sha256Str,
		})
		return nil
	})

	if err != nil {
		return err
	}
	jsonData, _ := json.MarshalIndent(entries, "", "  ")
	return os.WriteFile(indexPath, jsonData, 0644)
}

// ------------------------------
// 卸载逻辑
// ------------------------------
func uninstallFromAndroid() error {
	assetsDir := filepath.Join(androidRoot, androidAssetsPath)
	jniLibsDir := filepath.Join(androidRoot, androidJniLibsPath)
	libsDir := filepath.Join(androidRoot, androidLibsPath)

	fmt.Println("🗑️ Uninstalling and cleaning files...")

	// 1. 批量删除所有SO库（递归删除jniLibs下所有.so）
	if isDirExists(jniLibsDir) {
		err := filepath.Walk(jniLibsDir, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".so") {
				_ = os.Remove(path)
				fmt.Printf("✅ Deleted SO: %s\n", filepath.Base(path))
			}
			return nil
		})
		if err != nil {
			fmt.Printf("⚠️ Warning: failed to delete some SO files: %v\n", err)
		}
	}

	// 2. 删除SDL AAR
	if sdlAarPath != "" {
		aarFileName := filepath.Base(sdlAarPath)
		targetAarPath := filepath.Join(libsDir, aarFileName)
		if err := os.Remove(targetAarPath); err == nil {
			fmt.Printf("✅ Deleted SDL AAR: %s\n", targetAarPath)
		}
	} else {
		aarFiles, _ := filepath.Glob(filepath.Join(libsDir, "*.aar"))
		for _, f := range aarFiles {
			_ = os.Remove(f)
			fmt.Printf("✅ Deleted AAR: %s\n", f)
		}
	}

	// 3. 清理assets资源
	cleanTargets := []string{
		"LICENSE", "config.toml", "mods", "langs", indexFileName,
	}
	for _, target := range cleanTargets {
		targetPath := filepath.Join(assetsDir, target)
		_ = os.RemoveAll(targetPath)
	}

	return nil
}

// ------------------------------
// 工具函数
// ------------------------------
func isDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	sourceFile, _ := os.Open(src)
	defer sourceFile.Close()
	destFile, _ := os.Create(dst)
	defer destFile.Close()
	_, err := io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if skipHidden && f.IsDir() && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)
		if f.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		return copyFile(path, targetPath)
	})
}

func computeFileSHA256(filePath string) (string, error) {
	file, _ := os.Open(filePath)
	defer file.Close()
	hash := sha256.New()
	_, _ = io.Copy(hash, file)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
