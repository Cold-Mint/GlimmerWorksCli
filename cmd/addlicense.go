/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var agplHeaderTemplate = `/*
 * Copyright (C) %s  %s <%s>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 * 
 * 版权(C) %s  %s <%s>
 *
 * 本程序是自由软件：你可以遵照自由软件基金会出版的GNU Affero通用公共许可证条款来重新分发和修改它
 * 该许可证的第3版，或者（由你选择）任何后续版本。
 *
 * 本程序的发布目的是希望它能有用，但没有任何担保；甚至没有适销性或特定用途适用性的默示担保。
 * 有关详细细节，请参阅GNU Affero通用公共许可证。
 *
 * 你应该已经收到一份GNU Affero通用公共许可证的副本。如果没有，请查阅<https://www.gnu.org/licenses/>。
 */
`

var addlicenseCmd = &cobra.Command{
	Use:   "addlicense",
	Short: "Add AGPLv3 license header to source files",
	Long:  "Add official GNU AGPLv3 license header (Chinese-English bilingual) to .go/.h/.cpp/.c files.",
	Run:   runAddLicense,
}

func init() {
	rootCmd.AddCommand(addlicenseCmd)

	addlicenseCmd.Flags().StringP("path", "p", ".", "Target file or directory")
	addlicenseCmd.Flags().StringP("author", "a", "", "Author name (required)")
	addlicenseCmd.Flags().StringP("year", "y", "", "Copyright year (required)")
	addlicenseCmd.Flags().StringP("email", "e", "", "Author email (required)") // 新增邮箱参数

	// 标记必填参数
	_ = addlicenseCmd.MarkFlagRequired("author")
	_ = addlicenseCmd.MarkFlagRequired("year")
	_ = addlicenseCmd.MarkFlagRequired("email")
}

func runAddLicense(cmd *cobra.Command, args []string) {
	path, _ := cmd.Flags().GetString("path")
	author, _ := cmd.Flags().GetString("author")
	year, _ := cmd.Flags().GetString("year")
	email, _ := cmd.Flags().GetString("email") // 获取邮箱参数

	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("Error: invalid path: %v\n", err)
		return
	}

	// 处理单个文件
	if !info.IsDir() {
		addHeaderToFile(path, author, year, email)
		return
	}

	// 遍历目录批量处理
	_ = filepath.Walk(path, func(fpath string, f os.FileInfo, err error) error {
		if err != nil || f.IsDir() {
			return err
		}
		ext := filepath.Ext(fpath)
		allowed := map[string]bool{
			".go": true, ".h": true, ".cpp": true, ".hpp": true, ".c": true, ".cc": true,
		}
		if allowed[ext] {
			addHeaderToFile(fpath, author, year, email)
		}
		return nil
	})
}

// addHeaderToFile 核心逻辑：新增 email 参数
func addHeaderToFile(filePath, author, year, email string) {
	// 读取文件内容
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Failed to read %s: %v\n", filePath, err)
		return
	}
	content := string(contentBytes)

	// 已存在许可证，直接跳过
	if strings.Contains(content, "GNU Affero General Public License") {
		fmt.Printf("[SKIPPED] License exists: %s\n", filePath)
		return
	}

	// 生成标准许可证头（传入6个参数：年、作者、邮箱、年、作者、邮箱）
	header := fmt.Sprintf(agplHeaderTemplate, year, author, email, year, author, email)

	var newContent string
	// 查找 #pragma once，精准替换其上方所有内容
	pragmaPos := strings.Index(content, "#pragma once")
	if pragmaPos != -1 {
		newContent = header + content[pragmaPos:]
	} else {
		// 无 #pragma once，直接添加到文件头部
		newContent = header + content
	}

	// 写入文件
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		fmt.Printf("Failed to write %s: %v\n", filePath, err)
	} else {
		fmt.Printf("[SUCCESS] License added: %s\n", filePath)
	}
}
