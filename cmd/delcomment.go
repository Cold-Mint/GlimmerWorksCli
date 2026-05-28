/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"
)

var deleteCommentRegex = regexp.MustCompile(`(?s)//\s*\n// Created by .+ on .+\.\n//\s*`)

var delcommentCmd = &cobra.Command{
	Use:   "delcomment",
	Short: "Delete the auto-generated creation comment",
	Long: `Delete the fixed-format creation comment from source files.
Matching format:
Supports .go, .h, .c, .cpp, .hpp, .cc files (author and date are variable).`,
	Run: runDeleteComment,
}

var commentPath string

func init() {
	rootCmd.AddCommand(delcommentCmd)
	delcommentCmd.Flags().StringVarP(&commentPath, "path", "p", ".", "Target file or directory to process")
}

func runDeleteComment(cmd *cobra.Command, args []string) {
	info, err := os.Stat(commentPath)
	if err != nil {
		printError("invalid path: %v", err)
		return
	}

	// 处理单个文件
	if !info.IsDir() {
		processFile(commentPath)
		return
	}

	// 遍历目录处理所有支持的文件
	err = filepath.Walk(commentPath, func(path string, f os.FileInfo, err error) error {
		if err != nil || f.IsDir() {
			return err
		}
		// 只处理指定后缀的代码文件
		exts := map[string]bool{
			".go": true, ".h": true, ".cpp": true,
			".c": true, ".hpp": true, ".cc": true,
		}
		if exts[filepath.Ext(path)] {
			processFile(path)
		}
		return nil
	})

	if err != nil {
		printError("walk directory failed: %v", err)
	}
}

// processFile 处理单个文件，删除匹配的注释
func processFile(path string) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		printError("read file failed: %s | %v", path, err)
		return
	}
	strContent := string(content)

	// 无匹配内容，跳过
	if !deleteCommentRegex.MatchString(strContent) {
		printInfo("[SKIPPED] No matching comment: %s", path)
		return
	}

	// 替换删除匹配的注释
	newContent := deleteCommentRegex.ReplaceAllString(strContent, "")

	// 写入文件
	err = os.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		printError("write file failed: %s | %v", path, err)
	} else {
		printSuccess("[SUCCESS] Deleted comment: %s", path)
	}
}

// 日志工具函数
func printInfo(format string, args ...interface{}) {
	str := "[delcomment] " + fmt.Sprintf(format, args...)
	println(str)
}

func printSuccess(format string, args ...interface{}) {
	str := "[delcomment] " + fmt.Sprintf(format, args...)
	println(str)
}

func printError(format string, args ...interface{}) {
	str := "[delcomment] ERROR: " + fmt.Sprintf(format, args...)
	println(str)
}
