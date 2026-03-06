package meta

type FileExtraMeta struct {
	IncludePaths  []string // 解析的//@include(filePath)路径
	ContentBlocks []string // 解析的//@content...//@endContent内容块
}
