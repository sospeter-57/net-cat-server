package utils

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxInt int = 1<<63 - 1
	minInt int = -1 << 63
)

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// mulWithOverflowError multiplies 2 ints and returns an error if the result will overflow.
func mulWithOverflowError(a, b int) (int, error) {
	if (-1 <= a && a <= 1) || (-1 <= b && b <= 1) {
		return a * b, nil
	}

	if (a >= 0 && b >= 0 && a > maxInt/b) ||
		(a < 0 && b < 0 && a < -(minInt/b)) ||
		(a < 0 && b > minInt/a || b < 0 && a > minInt/b) {
		return 0, fmt.Errorf("multiplication overflow: %d*%d", a, b)
	}

	return a * b, nil
}

// addWithOverflowError adds 2 ints and returns an error if the result will overflow.
func addWithOverflowError(a, b int) (int, error) {
	if (a >= 0 && b >= 0 && a > maxInt-b) ||
		(a < 0 && b < 0 && a < minInt-b) {
		return 0, fmt.Errorf("addition overflow: %d+%d", a, b)
	}

	return a + b, nil
}

// Atoi converts the decimal number in the given string into an int.
func Atoi(numStr string) (int, error) {
	sign := 1

	if strings.HasPrefix(numStr, "-") || strings.HasPrefix(numStr, "+") {
		if numStr[0] == '-' {
			sign = -1
		}

		numStr = numStr[1:]
	}

	if len(numStr) < 1 {
		return 0, errors.New("no digits found in string")
	}

	numStr = strings.TrimLeft(numStr, "0")
	var err error
	num := 0

	for _, r := range numStr {
		if !isDigit(r) {
			return 0, fmt.Errorf("%q is not a decimal digit", r)
		}

		d := int(r - '0')

		if num, err = mulWithOverflowError(num, 10); err != nil {
			return 0, fmt.Errorf("integer overflow: %s", numStr)
		}

		if num, err = addWithOverflowError(num, d*sign); err != nil {
			return 0, fmt.Errorf("integer overflow: %s", numStr)
		}
	}

	return num, nil
}
