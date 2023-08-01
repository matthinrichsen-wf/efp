// Package efp (Excel Formula Parser) tokenize an Excel formula using an
// implementation of E. W. Bachtal's algorithm, found here:
// https://ewbi.blogs.com/develops/2004/12/excel_formula_p.html
//
// Go language version by Ri Xu: https://xuri.me
package efp

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	sciregex = regexp.MustCompile(`^[1-9]{1}(\.[0-9]+)?E{1}$`)

	errStrings  = map[string]struct{}{"#NULL!": {}, "#DIV/0!": {}, "#VALUE!": {}, "#REF!": {}, "#NAME?": {}, "#NUM!": {}, "#N/A": {}}
	compStrings = map[string]struct{}{">=": {}, "<=": {}, "<>": {}}
)

type TokenType int8
type TokenSubType int8

func (t TokenType) String() string {
	switch t {
	case TokenTypeUnset:
		return `TokenTypeUnset`
	case TokenTypeNoop:
		return `TokenTypeNoop`
	case TokenTypeOperand:
		return `TokenTypeOperand`
	case TokenTypeFunction:
		return `TokenTypeFunction`
	case TokenTypeSubexpression:
		return `TokenTypeSubexpression`
	case TokenTypeArgument:
		return `TokenTypeArgument`
	case TokenTypeOperatorPrefix:
		return `TokenTypeOperatorPrefix`
	case TokenTypeOperatorInfix:
		return `TokenTypeOperatorInfix`
	case TokenTypeOperatorPostfix:
		return `TokenTypeOperatorPostfix`
	case TokenTypeWhitespace:
		return `TokenTypeWhitespace`
	case TokenTypeUnknown:
		return `TokenTypeUnknown`
	}
	return `TokenTypeUnset`
}

func (t TokenSubType) String() string {
	switch t {
	case TokenSubTypeUnset:
		return `TokenSubTypeUnset`
	case TokenSubTypeStart:
		return `TokenSubTypeStart`
	case TokenSubTypeStop:
		return `TokenSubTypeStop`
	case TokenSubTypeText:
		return `TokenSubTypeText`
	case TokenSubTypeNumber:
		return `TokenSubTypeNumber`
	case TokenSubTypeLogical:
		return `TokenSubTypeLogical`
	case TokenSubTypeError:
		return `TokenSubTypeError`
	case TokenSubTypeRange:
		return `TokenSubTypeRange`
	case TokenSubTypeMath:
		return `TokenSubTypeMath`
	case TokenSubTypeConcatenation:
		return `TokenSubTypeConcatenation`
	case TokenSubTypeIntersection:
		return `TokenSubTypeIntersection`
	case TokenSubTypeUnion:
		return `TokenSubTypeUnion`
	}
	return `TokenSubTypeUnset`
}

// QuoteDouble, QuoteSingle and other's constants are token definitions.
const (
	// Character constants
	QuoteDouble  = "\""
	QuoteSingle  = "'"
	BracketClose = "]"
	BracketOpen  = "["
	BraceOpen    = "{"
	BraceClose   = "}"
	ParenOpen    = "("
	ParenClose   = ")"
	Semicolon    = ";"
	Whitespace   = " "
	Comma        = ","
	ErrorStart   = "#"

	OperatorsSN      = "+-"
	OperatorsInfix   = "+-*/^&=><"
	OperatorsPostfix = "%"

	// Token type
	TokenTypeUnset TokenType = iota
	TokenTypeNoop
	TokenTypeOperand
	TokenTypeFunction
	TokenTypeSubexpression
	TokenTypeArgument
	TokenTypeOperatorPrefix
	TokenTypeOperatorInfix
	TokenTypeOperatorPostfix
	TokenTypeWhitespace
	TokenTypeUnknown

	// Token subtypes
	TokenSubTypeUnset TokenSubType = iota
	TokenSubTypeStart
	TokenSubTypeStop
	TokenSubTypeText
	TokenSubTypeNumber
	TokenSubTypeLogical
	TokenSubTypeError
	TokenSubTypeRange
	TokenSubTypeMath
	TokenSubTypeConcatenation
	TokenSubTypeIntersection
	TokenSubTypeUnion
)

// Token encapsulate a formula token.
type Token struct {
	TValue   string
	TType    TokenType
	TSubType TokenSubType
}

// Tokens directly maps the ordered list of tokens.
// Attributes:
//
//    items - Ordered list
//    index - Current position in the list
//
type Tokens struct {
	Index int
	Items []Token
}

// Parser inheritable container. TokenStack directly maps a LIFO stack of
// tokens.
type Parser struct {
	Formula    string
	Tokens     Tokens
	TokenStack Tokens
	Offset     int
	Token      string
	InString   bool
	InPath     bool
	InRange    bool
	InError    bool
}

// fToken provides function to encapsulate a formula token.
func fToken(value string, tokenType TokenType, subType TokenSubType) Token {
	return Token{
		TValue:   value,
		TType:    tokenType,
		TSubType: subType,
	}
}

// fTokens provides function to handle an ordered list of tokens.
func fTokens(length, cap int) Tokens {
	return Tokens{
		Index: -1,
		Items: make([]Token, length, cap),
	}
}

// add provides function to add a token to the end of the list.
func (tk *Tokens) add(value string, tokenType TokenType, subType TokenSubType) Token {
	token := fToken(value, tokenType, subType)
	tk.addRef(token)
	return token
}

// addRef provides function to add a token to the end of the list.
func (tk *Tokens) addRef(token Token) {
	tk.Items = append(tk.Items, token)
}

// reset provides function to reset the index to -1.
func (tk *Tokens) reset() {
	tk.Index = -1
}

// BOF provides function to check whether beginning of list.
func (tk *Tokens) BOF() bool {
	return tk.Index <= 0
}

// EOF provides function to check whether end of list.
func (tk *Tokens) EOF() bool {
	return tk.Index >= (len(tk.Items) - 1)
}

// moveNext provides function to move the index along one.
func (tk *Tokens) moveNext() bool {
	if tk.EOF() {
		return false
	}
	tk.Index++
	return true
}

// current return the current token.
func (tk *Tokens) current() *Token {
	if tk.Index == -1 {
		return nil
	}
	return &tk.Items[tk.Index]
}

// next return the next token (leave the index unchanged).
func (tk *Tokens) next() *Token {
	if tk.EOF() {
		return nil
	}
	return &tk.Items[tk.Index+1]
}

// previous return the previous token (leave the index unchanged).
func (tk *Tokens) previous() *Token {
	if tk.Index < 1 {
		return nil
	}
	return &tk.Items[tk.Index-1]
}

// push provides function to push a token onto the stack.
func (tk *Tokens) push(token Token) {
	tk.Items = append(tk.Items, token)
}

// pop provides function to pop a token off the stack.
func (tk *Tokens) pop() Token {
	if len(tk.Items) == 0 {
		return Token{
			TType:    TokenTypeFunction,
			TSubType: TokenSubTypeStop,
		}
	}
	t := tk.Items[len(tk.Items)-1]
	tk.Items = tk.Items[:len(tk.Items)-1]
	return fToken("", t.TType, TokenSubTypeStop)
}

// token provides function to non-destructively return the top item on the
// stack.
func (tk *Tokens) token() *Token {
	if len(tk.Items) > 0 {
		return &tk.Items[len(tk.Items)-1]
	}
	return nil
}

// value return the top token's value.
func (tk *Tokens) value() string {
	if tk.token() == nil {
		return ""
	}
	return tk.token().TValue
}

// tp return the top token's type.
func (tk *Tokens) tp() TokenType {
	if tk.token() == nil {
		return TokenTypeUnset
	}
	return tk.token().TType
}

// subtype return the top token's subtype.
func (tk *Tokens) subtype() TokenSubType {
	if tk.token() == nil {
		return TokenSubTypeUnset
	}
	return tk.token().TSubType
}

// ExcelParser provides function to parse an Excel formula into a stream of
// tokens.
func ExcelParser() Parser {
	return Parser{}
}

// getTokens return a token stream (list).
func (ps *Parser) getTokens() Tokens {
	ps.Formula = strings.TrimSpace(ps.Formula)
	f := []rune(ps.Formula)
	if len(f) > 0 {
		if f[0] != '=' {
			ps.Formula = "=" + ps.Formula
		}
	}

	// state-dependent character evaluation (order is important)
	for !ps.EOF() {

		// double-quoted strings
		// embeds are doubled
		// end marks token
		if ps.InString {
			if ps.currentChar() == QuoteDouble {
				if ps.nextChar() == QuoteDouble {
					ps.Token += QuoteDouble
					ps.Offset++
				} else {
					ps.InString = false
					ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeText)
					ps.Token = ""
				}
			} else {
				ps.Token += ps.currentChar()
			}
			ps.Offset++
			continue
		}

		// single-quoted strings (links)
		// embeds are double
		// end does not mark a token
		if ps.InPath {
			if ps.currentChar() == QuoteSingle {
				if ps.nextChar() == QuoteSingle {
					ps.Token += QuoteSingle
					ps.Offset++
				} else {
					ps.InPath = false
				}
			} else {
				ps.Token += ps.currentChar()
			}
			ps.Offset++
			continue
		}

		// bracketed strings (range offset or linked workbook name)
		// no embeds (changed to "()" by Excel)
		// end does not mark a token
		if ps.InRange {
			if ps.currentChar() == BracketClose {
				ps.InRange = false
			}
			ps.Token += ps.currentChar()
			ps.Offset++
			continue
		}

		// error values
		// end marks a token, determined from absolute list of values
		if ps.InError {
			ps.Token += ps.currentChar()
			ps.Offset++
			if _, isErr := errStrings[ps.Token]; isErr {
				ps.InError = false
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeError)
				ps.Token = ""
			}
			continue
		}

		// scientific notation check
		if strings.ContainsAny(ps.currentChar(), OperatorsSN) && len(ps.Token) > 1 {
			if sciregex.MatchString(ps.Token) {
				ps.Token += ps.currentChar()
				ps.Offset++
				continue
			}
		}

		// independent character evaluation (order not important)
		// establish state-dependent character evaluations
		if ps.currentChar() == QuoteDouble {
			if len(ps.Token) > 0 {
				// not expected
				ps.Tokens.add(ps.Token, TokenTypeUnknown, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.InString = true
			ps.Offset++
			continue
		}

		if ps.currentChar() == QuoteSingle {
			if len(ps.Token) > 0 {
				// not expected
				ps.Tokens.add(ps.Token, TokenTypeUnknown, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.InPath = true
			ps.Offset++
			continue
		}

		if ps.currentChar() == BracketOpen {
			ps.InRange = true
			ps.Token += ps.currentChar()
			ps.Offset++
			continue
		}

		if ps.currentChar() == ErrorStart {
			if len(ps.Token) > 0 {
				// not expected
				ps.Tokens.add(ps.Token, TokenTypeUnknown, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.InError = true
			ps.Token += ps.currentChar()
			ps.Offset++
			continue
		}

		// mark start and end of arrays and array rows
		if ps.currentChar() == BraceOpen {
			if len(ps.Token) > 0 {
				// not expected
				ps.Tokens.add(ps.Token, TokenTypeUnknown, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.TokenStack.push(ps.Tokens.add("ARRAY", TokenTypeFunction, TokenSubTypeStart))
			ps.TokenStack.push(ps.Tokens.add("ARRAYROW", TokenTypeFunction, TokenSubTypeStart))
			ps.Offset++
			continue
		}

		if ps.currentChar() == Semicolon {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.addRef(ps.TokenStack.pop())
			ps.Tokens.add(Comma, TokenTypeArgument, TokenSubTypeUnset)
			ps.TokenStack.push(ps.Tokens.add("ARRAYROW", TokenTypeFunction, TokenSubTypeStart))
			ps.Offset++
			continue
		}

		if ps.currentChar() == BraceClose {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.addRef(ps.TokenStack.pop())
			ps.Tokens.addRef(ps.TokenStack.pop())
			ps.Offset++
			continue
		}

		// trim white-space
		if ps.currentChar() == Whitespace {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.add("", TokenTypeWhitespace, TokenSubTypeUnset)
			ps.Offset++
			for (ps.currentChar() == Whitespace) && (!ps.EOF()) {
				ps.Offset++
			}
			continue
		}

		// multi-character comparators
		if _, isComparison := compStrings[ps.doubleChar()]; isComparison {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.add(ps.doubleChar(), TokenTypeOperatorInfix, TokenSubTypeLogical)
			ps.Offset += 2
			continue
		}

		// standard infix operators
		if strings.ContainsAny(OperatorsInfix, ps.currentChar()) {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.add(ps.currentChar(), TokenTypeOperatorInfix, TokenSubTypeUnset)
			ps.Offset++
			continue
		}

		// standard postfix operators
		if ps.currentChar() == OperatorsPostfix {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.add(ps.currentChar(), TokenTypeOperatorPostfix, TokenSubTypeUnset)
			ps.Offset++
			continue
		}

		// start subexpression or function
		if ps.currentChar() == ParenOpen {
			if len(ps.Token) > 0 {
				ps.TokenStack.push(ps.Tokens.add(ps.Token, TokenTypeFunction, TokenSubTypeStart))
				ps.Token = ""
			} else {
				ps.TokenStack.push(ps.Tokens.add("", TokenTypeSubexpression, TokenSubTypeStart))
			}
			ps.Offset++
			continue
		}

		// function, subexpression, array parameters
		if ps.currentChar() == Comma {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			if ps.TokenStack.tp() != TokenTypeFunction {
				ps.Tokens.add(ps.currentChar(), TokenTypeOperatorInfix, TokenSubTypeUnion)
			} else {
				ps.Tokens.add(ps.currentChar(), TokenTypeArgument, TokenSubTypeUnset)
			}
			ps.Offset++
			continue
		}

		// stop subexpression
		if ps.currentChar() == ParenClose {
			if len(ps.Token) > 0 {
				ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
				ps.Token = ""
			}
			ps.Tokens.addRef(ps.TokenStack.pop())
			ps.Offset++
			continue
		}

		// token accumulation
		ps.Token += ps.currentChar()
		ps.Offset++
	}

	// dump remaining accumulation
	if len(ps.Token) > 0 {
		ps.Tokens.add(ps.Token, TokenTypeOperand, TokenSubTypeUnset)
	}

	// move all tokens to a new collection, excluding all unnecessary white-space tokens
	// switch infix "-" operator to prefix when appropriate, switch infix "+"
	// operator to noop when appropriate, identify operand and infix-operator
	// subtypes, pull "@" from in front of function names
	tokens2 := fTokens(0, len(ps.Tokens.Items))

	var previousType TokenType
	var previousSubType TokenSubType
	for ps.Tokens.moveNext() {
		token := ps.Tokens.current()

		if token.TType == TokenTypeWhitespace {
			if ps.Tokens.BOF() || ps.Tokens.EOF() {
			} else if prev := ps.Tokens.previous(); !(((prev.TType == TokenTypeFunction) && (prev.TSubType == TokenSubTypeStop)) || ((prev.TType == TokenTypeSubexpression) && (prev.TSubType == TokenSubTypeStop)) || (prev.TType == TokenTypeOperand)) {
			} else if next := ps.Tokens.next(); !(((next.TType == TokenTypeFunction) && (next.TSubType == TokenSubTypeStart)) || ((next.TType == TokenTypeSubexpression) && (next.TSubType == TokenSubTypeStart)) || (next.TType == TokenTypeOperand)) {
			} else {
				tk := fToken(token.TValue, TokenTypeOperatorInfix, TokenSubTypeIntersection)
				token = &tk
			}
		}

		switch {
		case token.TType == TokenTypeOperatorInfix && token.TValue == "-":
			if tokens2.BOF() {
				token.TType = TokenTypeOperatorPrefix
			} else if ((previousType == TokenTypeFunction) && (previousSubType == TokenSubTypeStop)) || ((previousType == TokenTypeSubexpression) && (previousSubType == TokenSubTypeStop)) || (previousType == TokenTypeOperatorPostfix) || (previousType == TokenTypeOperand) {
				token.TSubType = TokenSubTypeMath
			} else {
				token.TType = TokenTypeOperatorPrefix
			}
		case token.TType == TokenTypeOperatorInfix && token.TValue == "+":
			if tokens2.BOF() {
				token.TType = TokenTypeNoop
			} else if (previousType == TokenTypeFunction) && (previousSubType == TokenSubTypeStop) || ((previousType == TokenTypeSubexpression) && (previousSubType == TokenSubTypeStop) || (previousType == TokenTypeOperatorPostfix) || (previousType == TokenTypeOperand)) {
				token.TSubType = TokenSubTypeMath
			} else {
				token.TType = TokenTypeNoop
			}
		case token.TType == TokenTypeOperatorInfix && token.TSubType == TokenSubTypeUnset:
			if r := token.TValue[0]; r == '<' || r == '>' || r == '=' {
				token.TSubType = TokenSubTypeLogical
			} else if token.TValue == "&" {
				token.TSubType = TokenSubTypeConcatenation
			} else {
				token.TSubType = TokenSubTypeMath
			}
		case token.TType == TokenTypeOperand && token.TSubType == TokenSubTypeUnset:
			if _, err := strconv.ParseFloat(token.TValue, 64); err != nil {
				if (token.TValue == "TRUE") || (token.TValue == "FALSE") {
					token.TSubType = TokenSubTypeLogical
				} else {
					token.TSubType = TokenSubTypeRange
				}
			} else {
				token.TSubType = TokenSubTypeNumber
			}
		case token.TType == TokenTypeFunction:
			if (len(token.TValue) > 0) && token.TValue[0] == '@' {
				token.TValue = token.TValue[1:]
			}
		}

		previousType = token.TType
		previousSubType = token.TSubType
		if token.TType == TokenTypeNoop {
			continue
		}

		tokens2.addRef(Token{
			TValue:   token.TValue,
			TType:    token.TType,
			TSubType: token.TSubType,
		})
	}

	tokens2.reset()
	return tokens2
}

// doubleChar provides function to get two characters after the current
// position.
func (ps *Parser) doubleChar() string {
	if len([]rune(ps.Formula)) >= ps.Offset+2 {
		return string([]rune(ps.Formula)[ps.Offset : ps.Offset+2])
	}
	return ""
}

// currentChar provides function to get the character of the current position.
func (ps *Parser) currentChar() string {
	return string([]rune(ps.Formula)[ps.Offset])
}

// nextChar provides function to get the next character of the current position.
func (ps *Parser) nextChar() string {
	if len([]rune(ps.Formula)) >= ps.Offset+2 {
		return string([]rune(ps.Formula)[ps.Offset+1 : ps.Offset+2])
	}
	return ""
}

// EOF provides function to check whether end of tokens stack.
func (ps *Parser) EOF() bool {
	return ps.Offset >= len([]rune(ps.Formula))
}

// Parse provides function to parse formula as a token stream (list).
func (ps *Parser) Parse(formula string) []Token {
	ps.Formula = formula
	ps.Tokens = ps.getTokens()
	return ps.Tokens.Items
}

// PrettyPrint provides function to pretty the parsed result with the indented
// format.
func (ps *Parser) PrettyPrint() string {
	indent := 0
	output := ""
	for _, t := range ps.Tokens.Items {
		if t.TSubType == TokenSubTypeStop {
			indent--
		}
		for i := 0; i < indent; i++ {
			output += "\t"
		}
		output += t.TValue + " <" + t.TType.String() + "> <" + t.TSubType.String() + ">" + "\n"
		if t.TSubType == TokenSubTypeStart {
			indent++
		}
	}
	return output
}

// Render provides function to get formatted formula after parsed.
func (ps *Parser) Render() string {
	output := ""
	for _, t := range ps.Tokens.Items {
		if t.TType == TokenTypeFunction && t.TSubType == TokenSubTypeStart {
			output += t.TValue + ParenOpen
		} else if t.TType == TokenTypeFunction && t.TSubType == TokenSubTypeStop {
			output += ParenClose
		} else if t.TType == TokenTypeSubexpression && t.TSubType == TokenSubTypeStart {
			output += ParenOpen
		} else if t.TType == TokenTypeSubexpression && t.TSubType == TokenSubTypeStop {
			output += ParenClose
		} else if t.TType == TokenTypeOperand && t.TSubType == TokenSubTypeText {
			output += QuoteDouble + t.TValue + QuoteDouble
		} else if t.TType == TokenTypeOperatorInfix && t.TSubType == TokenSubTypeIntersection {
			output += Whitespace
		} else {
			output += t.TValue
		}
	}
	return output
}
