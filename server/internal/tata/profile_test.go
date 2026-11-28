package tata

import "testing"

func TestNormalizeGender(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"", ""},
		{"女", "女性"},
		{"女性", "女性"},
		{"男", "男性"},
		{"男性", "男性"},
		{"其他", "其他"},
		{"不愿透露", "不愿透露"},
	}
	for _, c := range cases {
		got, err := normalizeGender(c.in)
		if err != nil || got != c.out {
			t.Fatalf("normalizeGender(%q)=%q %v, want %q", c.in, got, err, c.out)
		}
	}
	if _, err := normalizeGender("未知"); err == nil {
		t.Fatal("expected invalid gender")
	}
}
