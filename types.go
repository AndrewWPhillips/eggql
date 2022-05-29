package eggql

// types.go has standard GraphQL types like "ID" as well as custom scalars like "Time"

import (
	"fmt"
	"time"

	"github.com/andrewwphillips/eggql/internal/field"
)

// ID is used when a standard GraphQL ID type is required.
// An ID can be used like any other scalar (Int, etc) as a field type, resolver argument type etc.
// It is typically used for a field that uniquely identifies an object, but it is up to the server
// to guarantee uniqueness. It is stored as a string but can be encoded from an integer or string.
type ID = field.ID

// TagHolder is used to declare a field with name "_" (underscore) in a struct to allow metadata (tags)
// to be attached to a struct.  (Metadata can only be attached to fields, so we use an "_" field
// to allow attaching metadata to the parent struct.)  This is currently just used to attach a
// comment to Go structs that are used to generate a "description" (in the GraphQL schema) for
// objects, interfaces and unions.  See the "Star Wars" tutorial for an example.
// Note: An empty struct will not add to the size of the containing struct if declared at the start.
type TagHolder struct{}

// Time is a custom scalar for representing a point in time
type Time time.Time

// timeFormat represents how a Time is encoded in a string
const timeFormat = time.RFC3339 // GraphQL spec says that any "Time" ext. scalar type should use this format (ISO-8601)

// UnmarshalEGGQL is called when eggql needs to decode a string to a Time
func (pt *Time) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse(timeFormat, in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time", err)
	}
	*pt = Time(tmp) // cast from time.Time to eggql.Time
	return nil
}

// MarshalEGGQL encodes a Time object to a string
func (t Time) MarshalEGGQL() (string, error) {
	return time.Time(t).Format(timeFormat), nil
}

// BigInt is a custom scalar for representing a big.Int
// Note that we embed a big.Int so that we can use the standard big.Int methods
type BigInt struct{ big.Int }

// UnmarshalEGGQL is called when eggql needs to decode a string to a BigInt
// Note that MarshalEGGQL is not needed to encode a BigInt (big.Int.String() is used)
func (bi *BigInt) UnmarshalEGGQL(in string) error {
	if err := bi.Int.UnmarshalText([]byte(in)); err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar BigInt", err)
	}
	return nil
}
