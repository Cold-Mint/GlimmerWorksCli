package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

type VariableDefinition struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// updepsCmd represents the updeps command
var updepsCmd = &cobra.Command{
	Use:   "updeps",
	Short: "Update dependencies in versions.cmake to latest stable tags",
	Long: `The "updeps" command scans the 'versions.cmake' file in the current directory,
parses the dependency definitions from comment lines in the format:

  #owner/repo@version

If the version is specified as 'latest', it queries the GitHub Releases API
to fetch the latest stable release tag (ignoring prerelease versions like beta or RC).

After processing, it regenerates 'versions.cmake' with the dependencies set using
CMake syntax:

  set(OWNER_REPO_VERSION "1.2.3")

This allows you to maintain dependency versions consistently and update them
automatically from GitHub.

Example usage:

  # update dependencies in the current directory
  GlimmerWorksCli updeps

Notes:

- Lines in versions.cmake that do not match the '#repo@version' pattern are ignored.
- Only stable releases are considered when fetching the latest tag.
- The generated variable names replace '/' with '_' and convert to uppercase, ending with '_VERSION'.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir, errorMessage := os.Getwd()
		if errorMessage != nil {
			fmt.Println(errorMessage)
			return
		}
		versionFile := filepath.Join(dir, "versions.cmake")
		content, err := os.ReadFile(versionFile)
		if err != nil {
			fmt.Println("Failed to read versions.cmake:", err)
			return
		}
		lines := strings.Split(string(content), "\n")
		var defs []VariableDefinition
		var builder strings.Builder
		builder.WriteString("#Variable definitions are generated through GlimmerWorksCli. Please do not edit them.\n")
		builder.WriteString("#The dependencies of the repository can be defined through comments.\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") && strings.Contains(line, "@") {
				builder.WriteString(line)
				builder.WriteString("\n")
				line = strings.TrimPrefix(line, "#")
				line = strings.TrimSpace(line)
				parts := strings.SplitN(line, "@", 2)
				if len(parts) != 2 {
					fmt.Println("Invalid dep line:", line)
					continue
				}
				repo := parts[0]
				version := parts[1]
				name := repoToVarName(repo)
				if version == "latest" {
					defs = append(defs, VariableDefinition{Name: name, Value: GetTheLatestTag(repo)})
				} else {
					defs = append(defs, VariableDefinition{Name: name, Value: version})
				}
			}
		}
		for _, def := range defs {
			builder.WriteString(fmt.Sprintf("set(%s \"%s\")\n", def.Name, def.Value))
		}
		writeErr := os.WriteFile(versionFile, []byte(builder.String()), 0644)
		if writeErr != nil {
			fmt.Println("Failed to write versions.cmake:", writeErr)
			return
		}
		fmt.Println("versions.cmake updated successfully")
	},
}

func repoToVarName(repo string) string {
	name := strings.ReplaceAll(repo, "/", "_")
	name = strings.ToUpper(name)
	name = name + "_VERSION"
	return name
}

func GetTheLatestTag(repo string) string {
	fmt.Println("Get Latest ", repo)
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Failed to fetch latest release:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("GitHub API returned status: %s\n", resp.Status)
		return ""
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Println("Failed to decode JSON:", err)
		return ""
	}

	return release.TagName
}

func init() {
	rootCmd.AddCommand(updepsCmd)
}
