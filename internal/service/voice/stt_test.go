package voice

import "testing"

func TestAppendListClear(t *testing.T) {
	s := New(Options{ProjectRoot: t.TempDir(), Python: "python3"})
	s.Append("s1", "你好", 12)
	s.Append("s1", "世界", 8)
	got := s.List("s1")
	if len(got) != 2 || got[0].Text != "你好" || got[1].Text != "世界" {
		t.Fatalf("unexpected list: %+v", got)
	}
	s.Clear("s1")
	if len(s.List("s1")) != 0 {
		t.Fatalf("expected empty after clear")
	}
}

func TestLastJSONLine(t *testing.T) {
	raw := "loading model\n{\"ok\": true, \"text\": \"hi\"}\n"
	if got := lastJSONLine(raw); got != `{"ok": true, "text": "hi"}` {
		t.Fatalf("got %q", got)
	}
}
