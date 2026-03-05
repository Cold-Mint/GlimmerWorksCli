package meta

type NamespaceMark struct {
	LineIdx   int    // 标记所在行号
	Namespace string // 命名空间名称（已标准化为xxx::）
}
