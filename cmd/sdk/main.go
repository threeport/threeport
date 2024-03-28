/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"github.com/threeport/threeport/cmd/sdk/cmd"
	_ "github.com/threeport/threeport/cmd/sdk/cmd/create"
	_ "github.com/threeport/threeport/cmd/sdk/cmd/gen"
)

func main() {
	cmd.Execute()
}
