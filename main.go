package main
import (
	"os"
	"github.com/alist-org/alist/v3/cmd"
)


func main() {
	os.Args = append(os.Args, "--server")
	cmd.Execute()
}
