package substitutions

import (
	"strings"

	"github.com/two-hundred/celerity/libs/blueprint/pkg/source"
)

type interpolationParseState struct {
	parentSourceStart  *source.Meta
	relativeLineInfo   *source.Meta
	relativeSubStart   *source.Meta
	parsed             []*StringOrSubstitution
	inPossibleSub      bool
	inStringLiteral    bool
	potentialSub       string
	potentialNonSubStr string
	prevChar           rune
	errors             []error
	outputLineInfo     bool
}

// ParseSubstitutionValues parses a string that can contain interpolated
// references.
func ParseSubstitutionValues(
	substitutionContext, value string,
	parentSourceStart *source.Meta,
	outputLineInfo bool,
) ([]*StringOrSubstitution, error) {
	// This is hand-rolled to account for the fact that string literals
	// are supported in the spec for substitutions and they can contain
	// the "${" and "}" syntax, trying to do string splitting won't catch
	// this and regular expressions without lookaheads can't be used to express
	// this.

	// There are no openings for substitutions, the value is just a string literal.
	// This may not be the case if the string literal contains the "${" in which case
	// it will be caught in the process of evaulating every character in sequence.
	if !strings.Contains(value, "${") {
		return []*StringOrSubstitution{
			{
				StringValue: &value,
				SourceMeta:  parentSourceStart,
			},
		}, nil
	}

	state := &interpolationParseState{
		// To alleviate the frustration of having to deal with only seeing
		// one error at a time per substitution, all errors are collected.
		// The trade-off will not be significant in most use cases as string
		// values that can contain substitutions are not expected to be very long.
		errors:            []error{},
		parsed:            []*StringOrSubstitution{},
		parentSourceStart: parentSourceStart,
		relativeLineInfo: &source.Meta{
			Line:   0,
			Column: 0,
		},
		relativeSubStart:   &source.Meta{},
		inPossibleSub:      false,
		inStringLiteral:    false,
		potentialSub:       "",
		potentialNonSubStr: "",
		prevChar:           ' ',
		outputLineInfo:     outputLineInfo,
	}

	// TODO: iterate over runes instead of bytes
	// decode rune increment by rune width
	for i := 0; i < len(value); i += 1 {
		isOpenSubBracket := checkOpenSubBracket(state, value, i)
		checkStringLiteral(state, value, i)
		isCloseSubBracket := checkCloseSubBracket(state, value, i, substitutionContext)

		state.prevChar = rune(value[i])
		if !isCloseSubBracket {
			state.potentialNonSubStr += string(value[i])
		}
		if state.inPossibleSub && !isOpenSubBracket {
			state.potentialSub += string(value[i])
		}
		updateLineInfo(state, value[i])
	}

	if len(state.potentialNonSubStr) > 0 {
		parentLine := 0
		if parentSourceStart != nil {
			parentLine = parentSourceStart.Line
		}

		parentColumn := 0
		if parentSourceStart != nil {
			parentColumn = parentSourceStart.Column
		}

		sourceMeta := (*source.Meta)(nil)
		if state.outputLineInfo {
			sourceMeta = &source.Meta{
				Line:   toAbsLine(parentLine, state.relativeLineInfo.Line),
				Column: toAbsColumn(parentColumn, state.relativeLineInfo.Column-len(state.potentialNonSubStr)),
			}
		}

		state.parsed = append(state.parsed, &StringOrSubstitution{
			StringValue: &state.potentialNonSubStr,
			SourceMeta:  sourceMeta,
		})
	}

	if len(state.errors) > 0 {
		return nil, prepareSubstitutionErrors(substitutionContext, state)
	}

	return state.parsed, nil
}

func prepareSubstitutionErrors(substitutionContext string, state *interpolationParseState) error {
	line := (*int)(nil)
	if state.parentSourceStart != nil {
		line = &state.parentSourceStart.Line
	}
	column := (*int)(nil)
	if state.parentSourceStart != nil {
		column = &state.parentSourceStart.Column
	}
	return errSubstitutions(
		substitutionContext,
		state.errors,
		line,
		column,
	)
}

func updateLineInfo(state *interpolationParseState, value byte) {
	if value == '\n' {
		state.relativeLineInfo.Line += 1
		state.relativeLineInfo.Column = 1
	} else {
		state.relativeLineInfo.Column += 1
	}
}

func checkOpenSubBracket(state *interpolationParseState, value string, i int) bool {
	isOpenSubBracket := state.prevChar == '$' && value[i] == '{' && !state.inStringLiteral
	if isOpenSubBracket {
		// Start of a substitution
		state.inPossibleSub = true
		state.relativeSubStart = &source.Meta{
			Line:   state.relativeLineInfo.Line,
			Column: state.relativeLineInfo.Column + 1,
		}
		nonSubStr := state.potentialNonSubStr[:len(state.potentialNonSubStr)-1]
		if len(nonSubStr) > 0 {
			sourceMeta := createStringValSourceMeta(state, nonSubStr)
			state.parsed = append(state.parsed, &StringOrSubstitution{
				StringValue: &nonSubStr,
				SourceMeta:  sourceMeta,
			})
		}
	}
	return isOpenSubBracket
}

func createStringValSourceMeta(state *interpolationParseState, stringVal string) *source.Meta {
	if !state.outputLineInfo {
		return nil
	}

	parentLine := 1
	if state.parentSourceStart != nil {
		parentLine = state.parentSourceStart.Line
	}

	parentCol := 1
	if state.parentSourceStart != nil {
		parentCol = state.parentSourceStart.Column
	}

	column := toAbsColumn(
		parentCol,
		// Subtract 1 to account for the "$" code point in the "${"
		// indicating a potential start of a substitution that leads
		// to us taking the previous string value as a string literal.
		state.relativeLineInfo.Column-len(stringVal)-1,
	)

	return &source.Meta{
		Line:   toAbsLine(parentLine, state.relativeLineInfo.Line),
		Column: column,
	}
}

func checkStringLiteral(state *interpolationParseState, value string, i int) {
	if value[i] == '"' && state.prevChar != '\\' && state.inPossibleSub {
		state.inStringLiteral = !state.inStringLiteral
	}
}

func checkCloseSubBracket(state *interpolationParseState, value string, i int, substitutionContext string) bool {
	isCloseSubBracket := value[i] == '}' && state.inPossibleSub && !state.inStringLiteral
	if isCloseSubBracket {
		// End of a substitution
		subSourceStart := toAbsSourceMeta(state.parentSourceStart, state.relativeSubStart)
		parsedSub, err := ParseSubstitution(
			substitutionContext,
			state.potentialSub,
			subSourceStart,
			state.outputLineInfo,
		)
		if err != nil {
			state.errors = append(state.errors, err)
		} else {
			sourceMeta := createSubstitutionSourceMeta(state)
			state.parsed = append(state.parsed, &StringOrSubstitution{
				SubstitutionValue: parsedSub,
				SourceMeta:        sourceMeta,
			})
		}
		state.potentialSub = ""
		state.potentialNonSubStr = ""
		state.inPossibleSub = false
	}
	return isCloseSubBracket
}

func createSubstitutionSourceMeta(state *interpolationParseState) *source.Meta {
	if !state.outputLineInfo {
		return nil
	}

	parentLine := 1
	if state.parentSourceStart != nil {
		parentLine = state.parentSourceStart.Line
	}

	parentCol := 1
	if state.parentSourceStart != nil {
		// Subtract 2 to account for "${" that allows accurate column position reporting
		// for the wrapper Substitution nodes at the top level of parsing
		// a substitution.
		// For example, if there is an error with the substitution as a whole, the column
		// reported should be the start of the "${" that wraps the substitution.
		// This would be reflected to the user by something like
		// range highlighting in an editor.
		parentCol = state.parentSourceStart.Column - 2
	}

	return &source.Meta{
		Line:   toAbsLine(parentLine, state.relativeSubStart.Line),
		Column: toAbsColumn(parentCol, state.relativeSubStart.Column),
	}
}

func toAbsSourceMeta(parentSourceStart, relativeSubStart *source.Meta) *source.Meta {
	if parentSourceStart == nil {
		return &source.Meta{
			Line:   relativeSubStart.Line + 1,
			Column: relativeSubStart.Column + 1,
		}
	}

	return &source.Meta{
		Line:   toAbsLine(parentSourceStart.Line, relativeSubStart.Line),
		Column: toAbsColumn(parentSourceStart.Column, relativeSubStart.Column),
	}
}

func toAbsColumn(parentColumn, relativeColumn int) int {
	return parentColumn + relativeColumn
}

func toAbsLine(parentLine, relativeLine int) int {
	return parentLine + relativeLine
}

// ParseSubstitution parses a string that represents a substitution
// that is the contents of an interpolated "${..}" block.
func ParseSubstitution(
	substitutionContext string,
	substitutionInput string,
	parentSourceStart *source.Meta,
	outputLineInfo bool,
) (*Substitution, error) {
	tokens, err := lex(substitutionInput, parentSourceStart)
	if err != nil {
		return nil, err
	}

	parser := NewParser(tokens, parentSourceStart, outputLineInfo)
	return parser.Parse()
}
