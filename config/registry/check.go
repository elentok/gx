package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// SizeParser parses a byte size such as "20G". The caller passes
// config.ParseByteSize; this package may not import config.
type SizeParser func(string) (int64, error)

// CheckValue checks one raw JSON value against the entry's type. It returns a
// short sentence the page shows after "Not saved: ", or nil. It never
// changes or clamps the value.
func CheckValue(e Entry, raw json.RawMessage, parseSize SizeParser) error {
	switch e.Type {
	case TypeBool:
		var v bool
		return decode(raw, &v, "Not true or false.")
	case TypeNumber:
		var v float64
		if err := decode(raw, &v, "Not a number."); err != nil {
			return err
		}
		return checkNumber(e, v)
	case TypeDuration:
		var s string
		if err := decode(raw, &s, "Not a duration. Use a number and a unit, for example 5m or 720h."); err != nil {
			return err
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return errors.New("Not a duration. Use a number and a unit, for example 5m or 720h.")
		}
		if d <= 0 {
			return errors.New("Duration must be above zero.")
		}
		return nil
	case TypeSize:
		var s string
		if err := decode(raw, &s, "Not a size. Use a number and a unit, for example 500MB or 20G."); err != nil {
			return err
		}
		if _, err := parseSize(s); err != nil {
			return errors.New("Not a size. Use a number and a unit, for example 500MB or 20G.")
		}
		return nil
	case TypeEnum:
		var s string
		if err := decode(raw, &s, "Not one of the allowed values."); err != nil {
			return err
		}
		if !slices.Contains(e.Enum, s) {
			return fmt.Errorf("Not one of the allowed values: %s.", strings.Join(e.Enum, ", "))
		}
		return nil
	case TypeString, TypeSecret:
		var s string
		return decode(raw, &s, "Not text.")
	case TypeStringList:
		var v []string
		return decode(raw, &v, "Not a list of text values.")
	case TypeNumberList:
		var v []float64
		if err := decode(raw, &v, "Not a list of numbers."); err != nil {
			return err
		}
		for _, n := range v {
			if err := checkNumber(e, n); err != nil {
				return err
			}
		}
		return nil
	case TypeChecks:
		var v map[string]json.RawMessage
		return decode(raw, &v, "Not an object.")
	}
	return fmt.Errorf("Unknown setting type.")
}

func decode(raw json.RawMessage, v any, reason string) error {
	if string(raw) == "null" || json.Unmarshal(raw, v) != nil {
		return errors.New(reason)
	}
	return nil
}

func checkNumber(e Entry, v float64) error {
	if e.Integer && v != math.Trunc(v) {
		return errors.New("Must be a whole number.")
	}
	if e.Min != nil && v < *e.Min {
		return fmt.Errorf("Must be at least %v.", *e.Min)
	}
	return nil
}
