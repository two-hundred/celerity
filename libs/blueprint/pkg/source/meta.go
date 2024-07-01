package source

// Meta represents information about the deserialised source of
// a blueprint value including the line and column
// where a blueprint element begins that can be used by tools such
// as linters to provide more detailed diagnostics to users creating
// blueprints from source in some supported formats.
type Meta struct {
	Line   int
	Column int
}
