package watchtower

import "testing"

func TestNormalizePaneTail(t *testing.T) {
	t.Parallel()

	got := normalizePaneTail("\n\n  a  \n\n b\n c\n d\n e \n")
	want := "b\nc\nd\ne"
	if got != want {
		t.Fatalf("normalizePaneTail = %q, want %q", got, want)
	}
}

func TestHashPaneTail(t *testing.T) {
	t.Parallel()

	a := hashPaneTail("hello")
	b := hashPaneTail("hello")
	c := hashPaneTail("world")
	if a == "" {
		t.Fatal("hashPaneTail should not return empty hash for non-empty input")
	}
	if a != b {
		t.Fatalf("hashPaneTail not deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("hashPaneTail should differ for different input: %q == %q", a, c)
	}
}
