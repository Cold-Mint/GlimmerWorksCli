package main

import "GlimmerWorksCli/cmd"

func main() {
	//When you need to manually test the command, edit this line of code.
	//当你需要手动测试命令时编辑这行代码。
	//os.Args = []string{"GlimmerWorksCli", "genCode", "-t", "2", "-d", "/home/coldmint/projects/GlimmerWorks", "-o", "/home/coldmint/projects/GlimmerWorks/core/utils"}
	cmd.Execute()
}
