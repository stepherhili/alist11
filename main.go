package main
package os
import "github.com/alist-org/alist/v3/cmd"

func main() {
	os.Args = append(os.Args, "--server")
	cmd.Execute()
}
