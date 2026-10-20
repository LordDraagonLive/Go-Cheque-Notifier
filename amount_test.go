package main

import "testing"

func TestAmountCompatibility(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"Rs5,000", "5000"}, {"Rs12,500.50", "12500.50"}, {"Rs0005.00", "5.00"},
		{"Rs-0.00", "0"}, {"Rs+.5", "0.5"}, {"Rs1.", "1"}, {"Rs-1,250.00", "-1250.00"},
		{"Rs0.000001", "0.000001"}, {"Rs0.0000001", "1E-7"}, {"Rs1e3", "1E+3"},
		{"Rs1.2300E2", "123.00"}, {"Rs1e-6144", "0"}, {"Rs1e-6143", "1E-6143"},
		{"Rs9.999999999999999999999999999999999e6144", "9.999999999999999999999999999999999E+6144"},
		{"Rs1.2345678901234567890123456789012345", "1.234567890123456789012345678901234"},
		{"Rs1.2345678901234567890123456789012355", "1.234567890123456789012345678901236"},
		{"Rs9.9999999999999999999999999999999995", "10.00000000000000000000000000000000"},
		{"5000", "0"},       // The parser strips two characters even without a currency prefix.
		{"💰€5,000", "5000"}, // Count Unicode code points rather than UTF-8 bytes.
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := transformAmount(tt.input)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	for _, input := range []string{"", "R", "Rs", "LKR 5,000", "Rs 5,000", "RsNaN", "RsInfinity", "Rs1e6145", "Rs1e2147483648", "Rs1.0e-2147483647"} {
		if _, err := transformAmount(input); err == nil {
			t.Errorf("%q should fail", input)
		}
	}
}
