package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	targetCMakeFile string
	dryRun          bool
)

// sortCppFileCmd represents the sortCppFile command
var sortCppFileCmd = &cobra.Command{
	Use:   "sortCppFile",
	Short: "Sort .cpp/.h/.c source file paths inside CMakeLists.txt",
	Long: `This tool parses CMakeLists.txt, extracts all source file entries with suffix .cpp/.h/.c,
sorts them with priority: .cpp first, then .c, finally .h, lexicographically inside same suffix,
and rewrites the file.
Paths containing variable symbol "$" will be wrapped with double quotes, normal paths without quotes.

Usage Examples:
  # Auto detect CMakeLists.txt in current working directory
  yourcli sortCppFile

  # Specify target CMake file via flag
  yourcli sortCppFile -f ./src/CMakeLists.txt

  # Dry run: print modified content only, no file write
  yourcli sortCppFile -f ./CMakeLists.txt --dry-run
`,
	Args: cobra.NoArgs, // Disallow positional arguments, use flag only
	Run: func(cmd *cobra.Command, args []string) {
		var cmakePath string
		if targetCMakeFile == "" {
			cmakePath = "CMakeLists.txt"
			fmt.Printf("No file path specified via -f flag, auto use file at current directory: %s\n", cmakePath)
		} else {
			cmakePath = targetCMakeFile
		}

		if err := sortCMakeSourceFiles(cmakePath); err != nil {
			fmt.Printf("Process failed: %v\n", err)
			os.Exit(1)
		}
		if !dryRun {
			fmt.Println("Success: source files sorted and written back to", cmakePath)
		} else {
			fmt.Println("Dry run finished, no file changes saved")
		}
	},
}

func init() {
	rootCmd.AddCommand(sortCppFileCmd)
	sortCppFileCmd.Flags().StringVarP(&targetCMakeFile, "file", "f", "", "File path to target CMakeLists.txt")
	sortCppFileCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Dry run mode, print output without writing to file")
}

// FileEntry stores single source file path metadata
type FileEntry struct {
	RawLine string // Formatted output line (with quotes processed)
	Path    string // Raw path without quotes, used for sorting
	HasVar  bool   // Mark whether path contains $ variable symbol
	Suffix  string // file suffix: .cpp / .c / .h
}

// getSuffixPriority return weight for sort priority: cpp=0, c=1, h=2
func getSuffixPriority(suffix string) int {
	switch suffix {
	case ".cpp":
		return 0
	case ".c":
		return 1
	case ".h":
		return 2
	default:
		return 999
	}
}

// sortCMakeSourceFiles core logic for parsing and sorting CMake source entries
func sortCMakeSourceFiles(filePath string) error {
	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("target file not accessible: %w", err)
	}
	if stat.IsDir() {
		return fmt.Errorf("%s is a directory, not a valid CMake file", filePath)
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	lines := strings.Split(string(contentBytes), "\n")

	var outputLines []string
	var collectingFiles []FileEntry
	inFileSet := false
	inSourceBlock := false

	for _, line := range lines {
		trimLine := strings.TrimSpace(line)

		if strings.Contains(trimLine, "FILES") {
			outputLines = append(outputLines, line)
			inFileSet = true
			continue
		}

		if trimLine == "PUBLIC" || trimLine == "PRIVATE" || trimLine == "INTERFACE" {
			outputLines = append(outputLines, line)
			inSourceBlock = true
			continue
		}

		if trimLine == "" || strings.HasSuffix(trimLine, ")") ||
			(strings.HasSuffix(trimLine, ":") && !strings.Contains(trimLine, "FILES")) {
			if len(collectingFiles) > 0 {
				// Core fix: sort by suffix priority first, then path lex order
				sort.Slice(collectingFiles, func(i, j int) bool {
					priI := getSuffixPriority(collectingFiles[i].Suffix)
					priJ := getSuffixPriority(collectingFiles[j].Suffix)
					if priI != priJ {
						return priI < priJ
					}
					// Same suffix, sort by path alphabet
					return collectingFiles[i].Path < collectingFiles[j].Path
				})
				for _, entry := range collectingFiles {
					outputLines = append(outputLines, "        "+entry.RawLine)
				}
				collectingFiles = collectingFiles[:0]
			}
			outputLines = append(outputLines, line)
			inFileSet = false
			inSourceBlock = false
			continue
		}

		if !inFileSet && !inSourceBlock {
			outputLines = append(outputLines, line)
			continue
		}

		lowerTrim := strings.ToLower(trimLine)
		var suffix string
		if strings.HasSuffix(lowerTrim, ".cpp") {
			suffix = ".cpp"
		} else if strings.HasSuffix(lowerTrim, ".c") {
			suffix = ".c"
		} else if strings.HasSuffix(lowerTrim, ".h") {
			suffix = ".h"
		} else {
			// Not source file, skip
			outputLines = append(outputLines, line)
			continue
		}

		rawTrim := trimLine
		hasVar := strings.Contains(rawTrim, "$")
		path := strings.Trim(rawTrim, `" `)

		var outputRaw string
		if hasVar {
			outputRaw = fmt.Sprintf(`"%s"`, path)
		} else {
			outputRaw = path
		}

		collectingFiles = append(collectingFiles, FileEntry{
			RawLine: outputRaw,
			Path:    path,
			HasVar:  hasVar,
			Suffix:  suffix,
		})
		continue
	}

	// Handle remaining files at file end
	if len(collectingFiles) > 0 {
		sort.Slice(collectingFiles, func(i, j int) bool {
			priI := getSuffixPriority(collectingFiles[i].Suffix)
			priJ := getSuffixPriority(collectingFiles[j].Suffix)
			if priI != priJ {
				return priI < priJ
			}
			return collectingFiles[i].Path < collectingFiles[j].Path
		})
		for _, entry := range collectingFiles {
			outputLines = append(outputLines, "        "+entry.RawLine)
		}
	}

	result := strings.Join(outputLines, "\n")

	if dryRun {
		fmt.Println("===== Dry Run Modified Content =====")
		fmt.Println(result)
		return nil
	}

	err = os.WriteFile(filePath, []byte(result), 0644)
	if err != nil {
		return fmt.Errorf("failed to write modified file: %w", err)
	}
	return nil
}
