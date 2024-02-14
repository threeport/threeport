package main

import (
	"flag"
	"fmt"
	"os"

	encryption "github.com/threeport/threeport/pkg/encryption/v0"
)

func main() {
	key := flag.String("key", "", "the encryption key")
	value := flag.String("value", "", "the value to decrypt")
	flag.Parse()

	if *key == "" || *value == "" {
		fmt.Println("Error: missing required argument")
		flag.PrintDefaults()
		os.Exit(1)
	}

	decryptedVal, err := encryption.Decrypt(*key, *value)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(decryptedVal)
}
