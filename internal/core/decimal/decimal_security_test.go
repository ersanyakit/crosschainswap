package decimal

import "testing"

func TestDecimalPrecisionDoesNotLeakFunds(t *testing.T) {
	// Arrange.
	left := "0.1"
	right := "0.2"

	// Act.
	sum := Add(left, right)
	remainder := SubFloorZero(sum, "0.3")

	// Assert.
	if sum != "0.3" {
		t.Fatalf("decimal sum = %s, want exact 0.3", sum)
	}
	if remainder != "0" {
		t.Fatalf("decimal remainder = %s, want exact zero", remainder)
	}
}

func TestDecimalRepeatedDustAddsExactly(t *testing.T) {
	// Arrange.
	total := "0"
	dust := "0.00000001"

	// Act.
	for i := 0; i < 10_000; i++ {
		total = Add(total, dust)
	}

	// Assert.
	if total != "0.0001" {
		t.Fatalf("dust total = %s, want exact 0.0001", total)
	}
}
