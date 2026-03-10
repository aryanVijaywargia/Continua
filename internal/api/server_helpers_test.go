package api

import "testing"

func TestNormalizePagination_Defaults(t *testing.T) {
	limit, offset := normalizePagination(nil, nil)

	if limit != defaultPageLimit {
		t.Fatalf("expected default limit %d, got %d", defaultPageLimit, limit)
	}
	if offset != 0 {
		t.Fatalf("expected default offset 0, got %d", offset)
	}
}

func TestNormalizePagination_ClampsBounds(t *testing.T) {
	tooLarge := 1_000_000
	negative := -42

	limit, offset := normalizePagination(&tooLarge, &negative)

	if limit != maxPageLimit {
		t.Fatalf("expected capped limit %d, got %d", maxPageLimit, limit)
	}
	if offset != 0 {
		t.Fatalf("expected clamped offset 0, got %d", offset)
	}
}

func TestNormalizePagination_ClampsNonPositiveLimit(t *testing.T) {
	zero := 0
	neg := -1

	limitZero, _ := normalizePagination(&zero, nil)
	if limitZero != 1 {
		t.Fatalf("expected limit 1 for zero input, got %d", limitZero)
	}

	limitNeg, _ := normalizePagination(&neg, nil)
	if limitNeg != 1 {
		t.Fatalf("expected limit 1 for negative input, got %d", limitNeg)
	}
}
