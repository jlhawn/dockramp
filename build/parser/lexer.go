package parser

import (
	"bufio"
	"fmt"
	"regexp"
)

type unevaluatedToken int

const (
	unevaluatedTokenWhitespace unevaluatedToken = iota
	unevaluatedTokenComment
	unevaluatedTokenNewline
	unevaluatedTokenDoubleQuotedString
	unevaluatedTokenSingleQuotedString
	unevaluatedTokenRawArg
)

// Double quoted escape sequences are handles specially if the escaped
// character is a backslash, a newline, or a double quote.
var dblQuoteEscapeSeq = regexp.MustCompile(`\\[\\\n"]`)

func dblQuoteEscapeFunc(esc string) string {
	switch esc[1] {
	case '\\':
		return `\` // Replace with single backslash.
	case '\n':
		return `` // Replace with nothing.
	case '"':
		return `"` // Replace with double quote.
	default:
		return esc // Don't replace anything.
	}
}

// Unquoted escape sequences should be replaced by the escaped character
// with the exception of newline in which case it is treated as a line
// continuation and is ignored.
var unquotedEscapeSeq = regexp.MustCompile(`\\[.\n]`)

func unquotedEscapeFunc(esc string) string {
	if esc[1] == '\n' {
		return "" // Replace with nothing.
	}

	return esc[1:]
}

// eval evaluates this unevaluated token and returns an evaluated token.
// Whitespace matches are evaluated to a whitespace token. Both Newline and
// Comment matches are evaluated to Newline token. Quoted tokens (backtick,
// single, or double quotes) are evaluated according to their escape rules. A
// raw arg token also goes through escape processing. If a token of unknown
// kind is processed, eval() panics.
func (t unevaluatedToken) eval(match string) token {
	switch t {
	case unevaluatedTokenWhitespace:
		// Whitespace yields whitespace.
		return whitespaceToken{}
	case unevaluatedTokenComment, unevaluatedTokenNewline:
		// Treat comments and newlines both as newlines.
		return newlineToken{}
	case unevaluatedTokenSingleQuotedString:
		// Single quoted strings are evaluated to the contents between the
		// quotes.
		return argToken(match[1 : len(match)-1])
	case unevaluatedTokenDoubleQuotedString:
		// Double quoted strings are evaluated to the contents between the
		// quotes with special escape processing:
		// 	\"			-> "
		// 	\\			-> \
		// 	\<newline>	-> <nothing>
		quoted := match[1 : len(match)-1]
		return argToken(dblQuoteEscapeSeq.ReplaceAllStringFunc(quoted, dblQuoteEscapeFunc))
	case unevaluatedTokenRawArg:
		// Args are the literal value with escaped sequences replaced
		// with the escaped character, except for escaped newlines which are
		// replaced with nothing.
		return argToken(unquotedEscapeSeq.ReplaceAllStringFunc(match, unquotedEscapeFunc))
	default:
		panic(fmt.Errorf("unevaluated token of unknown type: %q", match))
	}
}

type pattern struct {
	unevaluatedToken
	re *regexp.Regexp
}

var allPatterns = []pattern{
	{
		unevaluatedToken: unevaluatedTokenWhitespace,
		re:               regexp.MustCompile(`^([ \f\r\t\v]|\\\n)+`),
	},
	{
		unevaluatedToken: unevaluatedTokenComment,
		re:               regexp.MustCompile(`^#[^\n]*\n`),
	},
	{
		unevaluatedToken: unevaluatedTokenNewline,
		re:               regexp.MustCompile(`^\n+`),
	},
	{
		// Double quotes may contain double quotes escaped with a backslash.
		// When evaluated, escaped double quotes or backslashes are replaced by
		// a double quote or backslash and escaped newlines are replaced with
		// an empty string.
		unevaluatedToken: unevaluatedTokenDoubleQuotedString,
		re:               regexp.MustCompile(`^"(\\"|[^"])*"`),
	},
	{
		// Single quotes are like raw strings. They cannot contain another
		// single quote. No special escape processing is performed.
		unevaluatedToken: unevaluatedTokenSingleQuotedString,
		re:               regexp.MustCompile(`^'[^']*'`),
	},
	{
		unevaluatedToken: unevaluatedTokenRawArg,
		re:               regexp.MustCompile(`^([^<#'" \f\n\r\t\v\\]|\\.)+`),
	},
}

type unevaluatedHeredoc struct {
	ignoreLeadingTabs bool
	delimitingTerm    string
	matchPattern      *regexp.Regexp
}

var (
	heredocStartPattern = regexp.MustCompile(`^<<(-)?(?:[ \f\r\t\v])*(.+)\n`)
	leadingTabsPattern  = regexp.MustCompile(`(?m)^\t+`)
)

func (h *unevaluatedHeredoc) eval(match string) token {
	if h.ignoreLeadingTabs {
		match = leadingTabsPattern.ReplaceAllString(match, "")
	}

	return heredocToken(match)
}

func tokenize(currentToken *token) bufio.SplitFunc {
	var heredoc *unevaluatedHeredoc

	findHeredoc := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if len(data) == 0 && atEOF {
			// No more data to parse from stream.
			return 0, nil, fmt.Errorf("invalid heredoc at end of input: term %q", heredoc.delimitingTerm)
		}

		inputStr := string(data)

		// Check if the input matches the heredoc pattern.
		matches := heredoc.matchPattern.FindStringSubmatch(inputStr)
		if matches == nil {
			// No heredoc match found. If we're at EOF, then it's an invalid
			// heredoc.
			if atEOF {
				return 0, nil, fmt.Errorf("invalid heredoc at end of input: term %q", heredoc.delimitingTerm)
			}

			// Try to read more data so that we have more to match.
			return 0, nil, nil
		}

		fullMatch := matches[0]
		*currentToken = heredoc.eval(matches[1])

		// Trim the input.
		fullMatchBytes := []byte(fullMatch)

		// If there's not any remaining data, then that means the heredoc match
		// ended with the end of the input string (with or without a trailing
		// newline. If we are at EOF or the match did end with a newline then
		// this is a correct match. Otherwise, we need to attempt to read more
		// data to possibly get a longer match.
		if len(fullMatch) == len(inputStr) {
			if atEOF || matches[2] == "\n" {
				// We've found the full heredoc.
				heredoc = nil

				return len(fullMatchBytes), fullMatchBytes, nil
			}

			// The match did not end with a newline and there's more data
			// available from the input stream. Try to read more in case we can
			// match a longer heredoc. There could be more non-newline
			// characters after the current point.
			return 0, nil, nil
		}

		// Since there's more remaining input, then the match must have ended
		// with a newline.
		if matches[2] != "\n" {
			panic("heredoc match should have endede with a newline")
		}

		// We've found the full heredoc.
		heredoc = nil

		return len(fullMatchBytes), fullMatchBytes, nil
	}

	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// If we are in a heredoc, look for the end.
		if heredoc != nil {
			return findHeredoc(data, atEOF)
		}

		if len(data) == 0 && atEOF {
			// No more data to parse from stream.
			return 0, nil, nil
		}

		inputStr := string(data)

		// Check if the input matches the beginning of a heredoc.
		if matches := heredocStartPattern.FindStringSubmatch(inputStr); matches != nil {
			heredoc = &unevaluatedHeredoc{
				ignoreLeadingTabs: matches[1] == "-",
				delimitingTerm:    matches[2],
				matchPattern:      regexp.MustCompile(`^((?:.|\n)+?\n)?` + regexp.QuoteMeta(matches[2]) + `(\n|\z)`),
			}

			matchBytes := []byte(matches[0])

			advance, token, err = findHeredoc(data[len(matchBytes):], atEOF)

			return advance + len(matchBytes), token, err
		}

		var match string
		for _, pattern := range allPatterns {
			if match = pattern.re.FindString(inputStr); match == "" {
				continue // Try another pattern.
			}

			*currentToken = pattern.unevaluatedToken.eval(match)
			break
		}

		if match == "" { // No match found.
			if atEOF {
				return 0, nil, fmt.Errorf("invalid token at end of input with data=%q", inputStr)
			}

			// Try to read more data so we have more to match.
			return 0, nil, nil
		}

		if len(match) == len(inputStr) && !atEOF {
			// Try to read more data in case we can match a longer token.
			return 0, nil, nil
		}

		matchBytes := []byte(match)

		return len(matchBytes), matchBytes, nil
	}
}
