package main

import (
	"os"
	"github.com/alist-org/alist/v3/cmd"
)

func main() {
	// 设置第一个命令的参数
	os.Args = []string{"", "admin", "set", "1"}
	cmd.Execute()

	// 设置第二个命令的参数
	os.Args = []string{"", "server"}
	cmd.Execute()
}
