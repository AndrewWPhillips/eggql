package eggql

// time.go implements a GraphQL date/time type called "Time"

import (
	"fmt"
	"time"
)

const format = time.RFC3339 // GraphQL spec says that any "Time" ext. scalar type should use this format (ISO-8601)

type Time time.Time

// UnmarshalEGGQL is called when eggql needs to decode a string to a Time
func (pt *Time) UnmarshalEGGQL(in string) error {
	tmp, err := time.Parse(format, in)
	if err != nil {
		return fmt.Errorf("%w error in UnmarshalEGGQL for custom scalar Time", err)
	}
	*pt = Time(tmp)
	return nil
}

// MarshalEGGQL encodes a Time object to a string
func (t Time) MarshalEGGQL() (string, error) {
	return time.Time(t).Format(format), nil
}
