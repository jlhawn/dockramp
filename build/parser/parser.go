package parser

import (
	"bufio"
	"errors"
	"io"
)

// Command has arguments and an input literal from a heredoc.
type Command struct {
	Args    []string
	Heredoc string
}

// Parse parses the given input as a line-separated list of arguments.
// On success, a slice of argument lists is returned.
func Parse(input io.Reader) (commands []*Command, err error) {
	scanner := bufio.NewScanner(input)

	var currentToken token
	scanner.Split(tokenize(&currentToken))

	var tokens []token
	for scanner.Scan() {
		tokens = append(tokens, currentToken)

		if numTokens := len(tokens); numTokens > 1 {
			prevToken := tokens[numTokens-2]
			if mergedToken := prevToken.Merge(currentToken); mergedToken != nil {
				tokens[numTokens-2] = mergedToken
				tokens = tokens[:numTokens-1]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	beginning := true
	var currentCommand *Command
	for _, token := range tokens {
		if token.Type() == tokenTypeWhitespace {
			continue // Ignore whitespace tokens.
		}

		if token.Type() == tokenTypeNewline {
			if !beginning { // handle leading newlines.
				// Newline signals the end of a command.
				commands = append(commands, currentCommand)
				currentCommand = nil
			}
			continue
		}

		if token.Type() == tokenTypeHeredoc {
			if currentCommand == nil {
				return nil, errors.New("unexpected heredoc")
			}

			currentCommand.Heredoc = token.Value()

			// Heredoc also signals the end of a command.
			commands = append(commands, currentCommand)
			currentCommand = nil

			continue
		}

		if token.Type() != tokenTypeArg {
			return nil, errors.New("unexpected token")
		}

		beginning = false
		// Append arg to current command.
		if currentCommand == nil {
			currentCommand = &Command{}
		}
		currentCommand.Args = append(currentCommand.Args, token.Value())
	}

	if currentCommand != nil {
		// Handles case with no trailing newline.
		commands = append(commands, currentCommand)
	}

	return commands, nil
}
