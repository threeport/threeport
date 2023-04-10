package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	filePath := flag.String("file-path", "", "the file with contents to stringify")
	flag.Parse()

	if *filePath == "" {
		fmt.Println("Error: missing file path")
		flag.PrintDefaults()
		os.Exit(1)
	}

	content, err := ioutil.ReadFile(*filePath)
	if err != nil {
		panic(err)
	}

	strContent := strings.ReplaceAll(string(content), "\n", `\n`)

	fmt.Println(strContent)
}
