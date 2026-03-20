package utils

import "testing"

func mustNotPanicAtoi(t *testing.T, input string) (int, error) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Atoi(%q) panicked: %v", input, r)
		}
	}()

	return Atoi(input)
}

func TestAtoi(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  int
		expectErr bool
	}{
		{name: "empty", input: "", expected: 0, expectErr: true},
		{name: "zero", input: "0", expected: 0, expectErr: false},
		{name: "positive", input: "42", expected: 42, expectErr: false},
		{name: "negative", input: "-42", expected: -42, expectErr: false},
		{name: "explicit positive sign", input: "+42", expected: 42, expectErr: false},
		{name: "leading zeros", input: "000123", expected: 123, expectErr: false},
		{name: "negative leading zeros", input: "-000123", expected: -123, expectErr: false},
		{name: "plus zero", input: "+0", expected: 0, expectErr: false},
		{name: "minus zero", input: "-0", expected: 0, expectErr: false},
		{name: "spaces are invalid", input: " 42", expected: 0, expectErr: true},
		{name: "trailing spaces are invalid", input: "42 ", expected: 0, expectErr: true},
		{name: "embedded spaces are invalid", input: "4 2", expected: 0, expectErr: true},
		{name: "letters are invalid", input: "12a", expected: 0, expectErr: true},
		{name: "decimal point is invalid", input: "12.3", expected: 0, expectErr: true},
		{name: "double sign is invalid", input: "--1", expected: 0, expectErr: true},
		{name: "sign only plus", input: "+", expected: 0, expectErr: true},
		{name: "sign only minus", input: "-", expected: 0, expectErr: true},
		{name: "max int", input: "9223372036854775807", expected: maxInt, expectErr: false},
		{name: "min int", input: "-9223372036854775808", expected: minInt, expectErr: false},
		{name: "max int overflow", input: "9223372036854775808", expected: 0, expectErr: true},
		{name: "min int overflow", input: "-9223372036854775809", expected: 0, expectErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := mustNotPanicAtoi(t, tc.input)

			if (err != nil) != tc.expectErr {
				t.Fatalf("error mismatch for input %q: expected error=%v, got error=%v", tc.input, tc.expectErr, err != nil)
			}

			if output != tc.expected {
				t.Fatalf("result mismatch for input %q: expected %d, got %d", tc.input, tc.expected, output)
			}
		})
	}
}
