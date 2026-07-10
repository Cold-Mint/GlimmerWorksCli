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
	Short: "Sort .cpp/.c/.h source paths inside target_sources CMake blocks",
	Long: `This tool sorts source files in CMake target_sources blocks with strict rules:
1. Normal source blocks (PRIVATE/PUBLIC/INTERFACE, non FILE_SET):
   Order priority: .cpp first, then .c, sorted lexicographically within same suffix.
   Header files (.h) will be ignored in these blocks.
2. FILE_SET HEADERS FILES blocks:
   Only collect .h header files, sorted lexicographically; skip .cpp/.c sources.
3. Paths containing "$" variables will be wrapped with double quotes, plain paths no quotes.

Usage Examples:
  # Auto use CMakeLists.txt in current directory
  yourcli sortCppFile

  # Specify custom cmake file path
  yourcli sortCppFile -f ./src/CMakeLists.txt

  # Dry run, print modified content without writing disk
  yourcli sortCppFile --dry-run
`,
	Args: cobra.NoArgs,
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

// FileEntry single file metadata for sorting
type FileEntry struct {
	RawLine string
	Path    string
	Suffix  string
}

// getSourcePriority return weight for .cpp = 0, .c = 1
func getSourcePriority(suffix string) int {
	switch suffix {
	case ".cpp":
		return 0
	case ".c":
		return 1
	default:
		return 999
	}
}

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
	inFileSetHeaderBlock := false // inside FILE_SET HEADERS -> FILES
	inSourceBlock := false        // inside PRIVATE/PUBLIC/INTERFACE source block

	for _, line := range lines {
		trimLine := strings.TrimSpace(line)

		// Detect FILE_SET HEADERS block start
		if trimLine == "FILE_SET HEADERS" {
			outputLines = append(outputLines, line)
			continue
		}
		// Detect FILES under FILE_SET HEADERS
		if trimLine == "FILES" && len(outputLines) > 0 {
			lastLine := strings.TrimSpace(outputLines[len(outputLines)-1])
			if lastLine == "FILE_SET HEADERS" {
				outputLines = append(outputLines, line)
				inFileSetHeaderBlock = true
				inSourceBlock = false
				continue
			}
		}

		// Detect PRIVATE / PUBLIC / INTERFACE source block start
		if trimLine == "PRIVATE" || trimLine == "PUBLIC" || trimLine == "INTERFACE" {
			outputLines = append(outputLines, line)
			inSourceBlock = true
			inFileSetHeaderBlock = false
			continue
		}

		// Block terminate trigger: end parenthesis / new command line
		if trimLine == "" || strings.HasSuffix(trimLine, ")") || strings.HasSuffix(trimLine, ":") {
			// Sort collected files before exit block
			if len(collectingFiles) > 0 {
				if inSourceBlock {
					// Source block: sort by cpp > c, then path
					sort.Slice(collectingFiles, func(i, j int) bool {
						priI := getSourcePriority(collectingFiles[i].Suffix)
						priJ := getSourcePriority(collectingFiles[j].Suffix)
						if priI != priJ {
							return priI < priJ
						}
						return collectingFiles[i].Path < collectingFiles[j].Path
					})
				} else if inFileSetHeaderBlock {
					// Header block: only h, simple lex sort
					sort.Slice(collectingFiles, func(i, j int) bool {
						return collectingFiles[i].Path < collectingFiles[j].Path
					})
				}
				// Write sorted lines with fixed indent
				for _, entry := range collectingFiles {
					outputLines = append(outputLines, "        "+entry.RawLine)
				}
				collectingFiles = collectingFiles[:0]
			}
			outputLines = append(outputLines, line)
			inFileSetHeaderBlock = false
			inSourceBlock = false
			continue
		}

		// Not inside any collect block, write raw line
		if !inFileSetHeaderBlock && !inSourceBlock {
			outputLines = append(outputLines, line)
			continue
		}

		lowerTrim := strings.ToLower(trimLine)
		var suffix string
		var isValidFile = false

		// Match supported suffix
		if strings.HasSuffix(lowerTrim, ".cpp") {
			suffix = ".cpp"
			isValidFile = true
		} else if strings.HasSuffix(lowerTrim, ".c") {
			suffix = ".c"
			isValidFile = true
		} else if strings.HasSuffix(lowerTrim, ".h") {
			suffix = ".h"
			isValidFile = true
		}

		// Skip non-source lines
		if !isValidFile {
			outputLines = append(outputLines, line)
			continue
		}

		// Filter invalid suffix for current block
		if inSourceBlock && suffix == ".h" {
			outputLines = append(outputLines, line)
			continue
		}
		if inFileSetHeaderBlock && (suffix == ".cpp" || suffix == ".c") {
			outputLines = append(outputLines, line)
			continue
		}

		// Process quote wrap logic
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
			Suffix:  suffix,
		})
		continue
	}

	// Process remaining unflushed files at EOF
	if len(collectingFiles) > 0 {
		if inSourceBlock {
			sort.Slice(collectingFiles, func(i, j int) bool {
				priI := getSourcePriority(collectingFiles[i].Suffix)
				priJ := getSourcePriority(collectingFiles[j].Suffix)
				if priI != priJ {
					return priI < priJ
				}
				return collectingFiles[i].Path < collectingFiles[j].Path
			})
		} else if inFileSetHeaderBlock {
			sort.Slice(collectingFiles, func(i, j int) bool {
				return collectingFiles[i].Path < collectingFiles[j].Path
			})
		}
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
