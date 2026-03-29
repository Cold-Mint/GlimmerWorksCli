/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

// tomlKeyToSnakeCmd represents the tomlKeyToSnake command
var tomlKeyToSnakeCmd = &cobra.Command{
	Use:   "tomlKeyToSnake",
	Short: "Convert keys of all TOML files in current & subdirectories to snake_case",
	Long: `Recursively scan all .toml files in the current working directory and its subdirectories,
convert all keys from camelCase/CamelCase to snake_case,
and write the modified content back to the original files.
Supports all TOML structures: nested tables, array tables, inline tables, arrays.

Examples:
Original Key: packId, resourceKey, ServerPort
Converted Key: pack_id, resource_key, server_port`,
	Run: runTomlKeyToSnake,
}

func runTomlKeyToSnake(cmd *cobra.Command, args []string) {
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Failed to get current working directory: %v\n", err)
		return
	}
	fmt.Printf("🔍 Scanning directory: %s (including all subdirectories)\n", workDir)

	err = filepath.Walk(workDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			fmt.Printf("❌ Failed to access path [%s]: %v\n", path, walkErr)
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(info.Name()), ".toml") {
			if err := processSingleTomlFile(path); err != nil {
				fmt.Printf("❌ Failed to process file [%s]: %v\n", path, err)
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("❌ Failed to walk directory: %v\n", err)
		return
	}
	fmt.Println("\n🎉 All TOML files processed successfully!")
}

func processSingleTomlFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file")
	}

	var root interface{}
	if err := toml.Unmarshal(content, &root); err != nil {
		return fmt.Errorf("failed to parse TOML: %v", err)
	}

	convertTomlKeysRecursive(&root)

	newContent, err := toml.Marshal(root)
	if err != nil {
		return fmt.Errorf("failed to marshal TOML")
	}
	finalContent := strings.ReplaceAll(string(newContent), "'", "\"")
	if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write file")
	}

	fmt.Printf("✅ Processed successfully: %s\n", filePath)
	return nil
}

func convertTomlKeysRecursive(val *interface{}) {
	switch v := (*val).(type) {
	case map[string]interface{}:
		newMap := make(map[string]interface{}, len(v))
		for k, vv := range v {
			convertTomlKeysRecursive(&vv)
			newKey := camelToSnake(k)
			newMap[newKey] = vv
		}
		*val = newMap

	case []interface{}:
		for i := range v {
			convertTomlKeysRecursive(&v[i])
		}
	}
}

func camelToSnake(s string) string {
	var result strings.Builder
	for i, char := range s {
		if unicode.IsUpper(char) {
			if i > 0 && !unicode.IsUpper(rune(s[i-1])) {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(char))
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func init() {
	rootCmd.AddCommand(tomlKeyToSnakeCmd)
}
