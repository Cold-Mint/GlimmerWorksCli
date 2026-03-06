package meta

type ClassDependency struct {
	ClassName string   // 类名（如glimmer::ResourceRef）
	Deps      []string // 该类依赖的其他类名
}
