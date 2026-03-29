package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ystkfujii/tring/internal/app/apply"
	"github.com/ystkfujii/tring/internal/cli"
	_ "github.com/ystkfujii/tring/pkg/impl/bootstrap"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var ve apply.ValidationError
		if errors.As(err, &ve) {
			os.Exit(1)
		}
		os.Exit(2)
	}
}
