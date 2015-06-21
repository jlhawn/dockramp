package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jlhawn/dockramp/build/parser"
)

func main() {
	input := os.Stdin

	if len(os.Args) > 1 {
		var err error
		input, err = os.Open(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		defer input.Close()
	}

	commands, err := parser.Parse(input)
	if err != nil {
		log.Fatalf("unable to parse input: %s", err)
	}

	for _, cmd := range commands {
		fmt.Printf("%#v\n", cmd)
	}
}
