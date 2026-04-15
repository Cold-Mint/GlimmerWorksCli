/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"crypto/sha256"
	"encoding/hex"
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
	soFileName         = "libGlimmerWorks.so"   // 要拷贝的so库名称
)

// androidCmd represents the android command
var androidCmd = &cobra.Command{
	Use:   "android",
	Short: "Install/Uninstall GlimmerWorks components to Android project",
	Long: `One-click install GlimmerWorks SO library, resource files and SDL AAR to Android project, or uninstall and clean all installed files.
Support specifying operation type, Android project root directory, CMake build directory (required only for installation)`,
	Run: runAndroidCommand,
}

// 命令行参数变量
var (
	action      string // 操作：install/uninstall
	androidRoot string // 安卓项目根目录
	buildDir    string // CMake build目录（安装必填）
	arch        string // 指定SO架构
	skipHidden  bool   // 忽略.开头的隐藏文件夹，默认true
	sdlAarPath  string // 新增：SDL AAR包文件路径
)

func init() {
	rootCmd.AddCommand(androidCmd)

	// 注册命令行参数
	androidCmd.Flags().StringVarP(&action, "action", "a", "", "Required: Operation type [install|uninstall]")
	androidCmd.Flags().StringVarP(&androidRoot, "android-root", "r", "", "Required: Android project root directory")
	androidCmd.Flags().StringVarP(&buildDir, "build-dir", "b", "", "Required for install: CMake build directory")
	androidCmd.Flags().StringVarP(&arch, "arch", "s", "", "Required for install: Specify SO architecture (arm64-v8a/armeabi-v7a/x86/x86_64)")
	androidCmd.Flags().StringVarP(&sdlAarPath, "sdl-aar", "l", "", "Required for install: Path to SDL AAR package file")
	androidCmd.Flags().BoolVarP(&skipHidden, "skip-hidden", "k", true, "Skip hidden folders starting with . (default: true)")

	// 标记必填参数
	_ = androidCmd.MarkFlagRequired("action")
	_ = androidCmd.MarkFlagRequired("android-root")
}

// 核心执行函数
func runAndroidCommand(cmd *cobra.Command, args []string) {
	// 1. 参数基础校验
	action = strings.ToLower(action)
	if action != "install" && action != "uninstall" {
		fmt.Printf("❌ Error: action parameter can only be install or uninstall\n")
		os.Exit(1)
	}

	// 校验安卓项目目录
	if !isDirExists(androidRoot) {
		fmt.Printf("❌ Error: Android project directory does not exist -> %s\n", androidRoot)
		os.Exit(1)
	}

	// 2. 执行对应逻辑
	switch action {
	case "install":
		// 安装：校验所有必填参数
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
// 安装逻辑实现
// ------------------------------
func installToAndroid() error {
	assetsDir := filepath.Join(androidRoot, androidAssetsPath)
	jniLibsDir := filepath.Join(androidRoot, androidJniLibsPath)
	libsDir := filepath.Join(androidRoot, androidLibsPath)

	// 1. 创建所有目标目录
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}
	if err := os.MkdirAll(jniLibsDir, 0755); err != nil {
		return fmt.Errorf("failed to create jniLibs directory: %w", err)
	}
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		return fmt.Errorf("failed to create libs directory: %w", err)
	}

	// 2. 拷贝so库
	fmt.Println("🔍 Copying SO library...")
	if err := copySOFile(jniLibsDir); err != nil {
		return err
	}

	// 3. 拷贝SDL AAR包
	fmt.Println("📦 Copying SDL AAR package...")
	aarFileName := filepath.Base(sdlAarPath)
	targetAarPath := filepath.Join(libsDir, aarFileName)
	if err := copyFile(sdlAarPath, targetAarPath); err != nil {
		return fmt.Errorf("failed to copy SDL AAR: %w", err)
	}

	// 4. 拷贝资源文件到assets
	fmt.Println("📂 Copying resource files...")
	resourceFiles := []string{"LICENSE", "config.toml"}
	resourceDirs := []string{"mods", "langs"}

	// 拷贝单个文件
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

	// 拷贝文件夹（递归，自动忽略隐藏文件夹）
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

	// 5. 生成index.toml
	fmt.Println("📝 Generating assets index file...")
	indexPath := filepath.Join(assetsDir, "index.toml")
	if err := generateIndexToml(assetsDir, indexPath); err != nil {
		return fmt.Errorf("failed to generate index.toml: %w", err)
	}

	return nil
}

// 拷贝SO库
func copySOFile(targetJniLibsDir string) error {
	soSourcePath := filepath.Join(buildDir, soFileName)
	if !isFileExists(soSourcePath) {
		return fmt.Errorf("SO library not found in build directory: %s", soSourcePath)
	}

	targetArchDir := filepath.Join(targetJniLibsDir, arch)
	if err := os.MkdirAll(targetArchDir, 0755); err != nil {
		return fmt.Errorf("failed to create architecture directory %s: %w", arch, err)
	}

	targetSoPath := filepath.Join(targetArchDir, soFileName)
	if err := copyFile(soSourcePath, targetSoPath); err != nil {
		return fmt.Errorf("failed to copy %s architecture SO: %w", arch, err)
	}
	fmt.Printf("✅ Copied [%s] architecture SO library\n", arch)
	return nil
}

// 生成index.toml（忽略隐藏文件夹）
func generateIndexToml(assetsDir, indexPath string) error {
	file, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return filepath.Walk(assetsDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == indexPath {
			return nil
		}

		// 忽略隐藏文件夹
		if skipHidden && f.IsDir() && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}

		relPath, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		isFile := !f.IsDir()
		sha256Str := ""
		if isFile {
			sha256Str, err = computeFileSHA256(path)
			if err != nil {
				return err
			}
		}

		entry := fmt.Sprintf(`[entry]
is_file = %t
path = "%s"
sha256 = "%s"

`, isFile, relPath, sha256Str)
		_, err = file.WriteString(entry)
		return err
	})
}

// ------------------------------
// 卸载逻辑实现
// ------------------------------
func uninstallFromAndroid() error {
	assetsDir := filepath.Join(androidRoot, androidAssetsPath)
	jniLibsDir := filepath.Join(androidRoot, androidJniLibsPath)
	libsDir := filepath.Join(androidRoot, androidLibsPath)

	fmt.Println("🗑️ Uninstalling and cleaning files...")

	// 1. 删除所有so库
	soPattern := filepath.Join(jniLibsDir, "*", soFileName)
	soFiles, _ := filepath.Glob(soPattern)
	for _, f := range soFiles {
		if err := os.Remove(f); err == nil {
			fmt.Printf("✅ Deleted: %s\n", f)
		}
	}

	// 2. 删除SDL AAR包（匹配文件名删除）
	if sdlAarPath != "" {
		aarFileName := filepath.Base(sdlAarPath)
		targetAarPath := filepath.Join(libsDir, aarFileName)
		if err := os.Remove(targetAarPath); err == nil {
			fmt.Printf("✅ Deleted SDL AAR: %s\n", targetAarPath)
		}
	} else {
		// 未指定路径时，删除libs目录下所有aar（兼容）
		aarFiles, _ := filepath.Glob(filepath.Join(libsDir, "*.aar"))
		for _, f := range aarFiles {
			_ = os.Remove(f)
			fmt.Printf("✅ Deleted AAR: %s\n", f)
		}
	}

	// 3. 删除assets下的所有文件/文件夹
	cleanTargets := []string{
		"LICENSE",
		"config.toml",
		"index.toml",
		"mods",
		"langs",
	}

	for _, target := range cleanTargets {
		targetPath := filepath.Join(assetsDir, target)
		if err := os.RemoveAll(targetPath); err == nil {
			fmt.Printf("✅ Cleaned: %s\n", targetPath)
		}
	}

	return nil
}

// ------------------------------
// 通用工具函数
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
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 忽略隐藏文件夹
		if skipHidden && f.IsDir() && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)

		if f.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		return copyFile(path, targetPath)
	})
}

func computeFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
