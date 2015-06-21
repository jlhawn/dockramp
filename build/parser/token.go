package parser

type tokenType int

const (
	tokenTypeArg tokenType = iota
	tokenTypeWhitespace
	tokenTypeHeredoc
	tokenTypeNewline
)

type token interface {
	Type() tokenType
	Value() string
	// Merge should attempt to merge this token with the given next token. The
	// tokens should already be evaluated. If the tokens cannot be merged then
	// Merge returns nil.
	Merge(next token) token
}

type argToken string

func (t argToken) Type() tokenType {
	return tokenTypeArg
}

func (t argToken) Value() string {
	return string(t)
}

func (t argToken) Merge(next token) token {
	if next.Type() != tokenTypeArg {
		return nil
	}

	return argToken(string(t) + next.Value())
}

type whitespaceToken struct{}

func (t whitespaceToken) Type() tokenType {
	return tokenTypeWhitespace
}

func (t whitespaceToken) Value() string {
	return ""
}

func (t whitespaceToken) Merge(next token) token {
	switch next.Type() {
	case tokenTypeWhitespace:
		return whitespaceToken{}
	case tokenTypeNewline:
		return newlineToken{}
	default:
		return nil
	}
}

type newlineToken struct{}

func (t newlineToken) Type() tokenType {
	return tokenTypeNewline
}

func (t newlineToken) Value() string {
	return ""
}

func (t newlineToken) Merge(next token) token {
	switch next.Type() {
	case tokenTypeWhitespace, tokenTypeNewline:
		return newlineToken{}
	default:
		return nil
	}
}

type heredocToken string

func (t heredocToken) Type() tokenType {
	return tokenTypeHeredoc
}

func (t heredocToken) Value() string {
	return string(t)
}

func (t heredocToken) Merge(next token) token {
	switch next.Type() {
	case tokenTypeWhitespace, tokenTypeNewline:
		return t
	default:
		return nil
	}
}
