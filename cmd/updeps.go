package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

type GitHubRef struct {
	Object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"object"`
}

type GitHubTag struct {
	Object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"object"`
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
CMake syntax, where the value is the commit hash corresponding to the tag:

  set(OWNER_REPO_VERSION "commit_hash")

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
		dir, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			return
		}
		versionFile := filepath.Join(dir, "versions.cmake")
		content, err := os.ReadFile(versionFile)
		if err != nil {
			fmt.Println("Failed to read versions.cmake:", err)
			return
		}
		lines := strings.Split(string(content), "\n")

		var builder strings.Builder
		// 写入固定头部注释
		builder.WriteString("#Variable definitions are generated through GlimmerWorksCli. Please do not edit them.\n")
		builder.WriteString("#The dependencies of the repository can be defined through comments.\n")
		// 写入当前时间（精确到秒）
		currentTime := time.Now().Format("2006-01-02 15:04:05")
		builder.WriteString(fmt.Sprintf("#Last updated date: %s\n", currentTime))

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "@") {
				continue
			}
			// 保留原始注释行
			builder.WriteString(trimmed)
			builder.WriteString("\n")

			// 解析 owner/repo@version
			trimmed = strings.TrimPrefix(trimmed, "#")
			trimmed = strings.TrimSpace(trimmed)
			parts := strings.SplitN(trimmed, "@", 2)
			if len(parts) != 2 {
				fmt.Printf("Invalid dep line: %s\n", line)
				continue
			}
			repo := parts[0]
			version := parts[1]

			// 获取标签名和对应的提交哈希
			tag, commit, err := getTagAndCommit(repo, version)
			if err != nil {
				fmt.Printf("Failed to get tag/commit for %s@%s: %v\n", repo, version, err)
				// 可以选择继续或退出，这里继续但设置空值
				tag = version // 退而使用版本作为标签标注
				commit = ""
			}
			annotation := fmt.Sprintf("#%s-%s", repo, tag)
			builder.WriteString(annotation)
			builder.WriteString("\n")

			// 写入 set 语句，值为提交哈希（如果获取失败则为空字符串）
			varName := repoToVarName(repo)
			builder.WriteString(fmt.Sprintf("set(%s \"%s\")\n", varName, commit))
		}

		// 写入文件
		err = os.WriteFile(versionFile, []byte(builder.String()), 0644)
		if err != nil {
			fmt.Println("Failed to write versions.cmake:", err)
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

// getTagAndCommit 获取指定仓库的标签名和对应的提交哈希。
// 如果 version 为 "latest"，则先获取最新 release 的标签名。
// 然后查询该标签对应的 commit SHA（支持轻量标签和附注标签）。
func getTagAndCommit(repo, version string) (tag string, commit string, err error) {
	// 1. 确定标签名
	if version == "latest" {
		release, err := getLatestRelease(repo)
		if err != nil {
			return "", "", fmt.Errorf("get latest release: %w", err)
		}
		tag = release.TagName
	} else {
		tag = version
	}

	// 2. 获取该标签对应的 commit SHA
	commit, err = getCommitForTag(repo, tag)
	if err != nil {
		return tag, "", fmt.Errorf("get commit for tag %s: %w", tag, err)
	}
	return tag, commit, nil
}

// getLatestRelease 获取仓库的最新稳定 release（忽略草稿和预发布）
func getLatestRelease(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %s", resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	return &release, nil
}

// getCommitForTag 通过 GitHub Git Data API 获取标签对应的 commit SHA。
func getCommitForTag(repo, tag string) (string, error) {
	// 先获取 refs/tags/{tag}
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/refs/tags/%s", repo, tag)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("HTTP request for ref: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %s for ref", resp.Status)
	}

	var ref GitHubRef
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return "", fmt.Errorf("decode ref JSON: %w", err)
	}

	// 如果对象是 commit，直接返回 SHA
	if ref.Object.Type == "commit" {
		return ref.Object.SHA, nil
	}

	// 如果对象是 tag（附注标签），需要获取 tag 对象来得到 commit SHA
	if ref.Object.Type == "tag" {
		tagObjURL := fmt.Sprintf("https://api.github.com/repos/%s/git/tags/%s", repo, ref.Object.SHA)
		resp2, err := http.Get(tagObjURL)
		if err != nil {
			return "", fmt.Errorf("HTTP request for tag object: %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GitHub API returned status %s for tag object", resp2.Status)
		}

		var tagObj GitHubTag
		if err := json.NewDecoder(resp2.Body).Decode(&tagObj); err != nil {
			return "", fmt.Errorf("decode tag object JSON: %w", err)
		}
		if tagObj.Object.Type == "commit" {
			return tagObj.Object.SHA, nil
		}
		return "", fmt.Errorf("tag object does not point to a commit (type=%s)", tagObj.Object.Type)
	}

	return "", fmt.Errorf("unexpected object type for ref: %s", ref.Object.Type)
}

func init() {
	rootCmd.AddCommand(updepsCmd)
}
