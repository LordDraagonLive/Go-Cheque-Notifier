package main

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^([+-]?)([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?$`)

func transformAmount(amount string) (string, error) {
	runes := []rune(amount)
	if len(runes) < 2 {
		return "", errors.New("amount must contain a two-character prefix followed by a decimal")
	}
	return decimalString(strings.ReplaceAll(string(runes[2:]), ",", ""))
}

// decimalString mirrors Ballerina's DECIMAL128 conversion and BigDecimal rendering:
// 34 significant digits, ties to even, retained scale, and canonical zero.
func decimalString(value string) (string, error) {
	parts := decimalPattern.FindStringSubmatch(value)
	if parts == nil {
		return "", errors.New("invalid decimal amount after removing the first two characters and commas")
	}
	exponent := int64(0)
	if parts[3] != "" {
		var err error
		exponent, err = strconv.ParseInt(parts[3], 10, 32)
		if err != nil {
			return "", errors.New("decimal exponent is out of range")
		}
	}
	mantissa := parts[2]
	scale := -exponent
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		scale += int64(len(mantissa) - dot - 1)
		mantissa = strings.ReplaceAll(mantissa, ".", "")
	}
	if scale < -2147483648 || scale > 2147483647 {
		return "", errors.New("decimal scale is out of range")
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", nil
	}
	if len(digits) > 34 {
		discarded := digits[34:]
		retained := digits[:34]
		roundUp := discarded[0] > '5' || (discarded[0] == '5' &&
			(strings.Trim(discarded[1:], "0") != "" || (retained[33]-'0')%2 == 1))
		scale -= int64(len(discarded))
		digits = retained
		if roundUp {
			number, _ := new(big.Int).SetString(digits, 10)
			digits = number.Add(number, big.NewInt(1)).String()
			if len(digits) > 34 {
				digits = digits[:34]
				scale--
			}
		}
	}
	adjustedExponent := int64(len(digits)) - scale - 1
	if adjustedExponent > 6144 {
		return "", errors.New("decimal amount exceeds DECIMAL128 range")
	}
	if adjustedExponent < -6143 {
		return "0", nil
	}
	sign := ""
	if parts[1] == "-" {
		sign = "-"
	}
	if scale >= 0 && adjustedExponent >= -6 {
		point := int64(len(digits)) - scale
		if point <= 0 {
			return sign + "0." + strings.Repeat("0", int(-point)) + digits, nil
		}
		if scale == 0 {
			return sign + digits, nil
		}
		return sign + digits[:point] + "." + digits[point:], nil
	}
	coefficient := digits[:1]
	if len(digits) > 1 {
		coefficient += "." + digits[1:]
	}
	return fmt.Sprintf("%s%sE%+d", sign, coefficient, adjustedExponent), nil
}
