package tata

import "testing"

func TestDailyCardCopyMatchesDeck(t *testing.T) {
	if len(dailyCardCopies) != 78 {
		t.Fatalf("copies %d", len(dailyCardCopies))
	}
	for _, card := range deck() {
		item, ok := dailyCardCopies[card.Name]
		if !ok {
			t.Fatalf("missing copy %s", card.Name)
		}
		if item.UprightMessage == "" || item.ReversedMessage == "" || item.UprightYi == "" || item.ReversedYi == "" {
			t.Fatalf("empty copy %s", card.Name)
		}
	}
}

func TestDailyLibraryCopyUsesOrientation(t *testing.T) {
	cfg := map[string]string{
		cardYiKey("愚者"):                       "运营覆盖",
		cardCopyKey("愚者", "upright_message"):  "正位寄语",
		cardCopyKey("愚者", "upright_yi"):       "今日宜：正位",
		cardCopyKey("愚者", "reversed_message"): "逆位寄语",
		cardCopyKey("愚者", "reversed_yi"):      "今日宜：逆位",
	}
	msg, yi := dailyLibraryCopy(cfg, "愚者", "")
	if msg != "正位寄语" || yi != "运营覆盖" {
		t.Fatalf("upright %#v %#v", msg, yi)
	}
	msg, yi = dailyLibraryCopy(cfg, "愚者", "reversed")
	if msg != "逆位寄语" || yi != "今日宜：逆位" {
		t.Fatalf("reversed %#v %#v", msg, yi)
	}
}
