package cache

import "testing"

func TestLocalCacheRoundTrip(t *testing.T) {
	c, err := NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fp := "abcdef0123456789"

	if ok, _ := c.Has(fp); ok {
		t.Fatal("expected miss before save")
	}
	if err := c.Save(Entry{Fingerprint: fp, Module: "cards", Stage: "test", Conclusion: "success", SavedAtUnix: 100}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.Has(fp); !ok {
		t.Fatal("expected hit after save")
	}
	got, ok, err := c.Restore(fp)
	if err != nil || !ok {
		t.Fatalf("restore: ok=%v err=%v", ok, err)
	}
	if got.Module != "cards" || got.Stage != "test" || got.Conclusion != "success" {
		t.Errorf("restored entry = %+v", got)
	}
}

func TestSaveRequiresFingerprint(t *testing.T) {
	c, _ := NewLocalCache(t.TempDir())
	if err := c.Save(Entry{}); err == nil {
		t.Error("expected error saving entry without a fingerprint")
	}
}

func TestNoopCache(t *testing.T) {
	var c Cache = Noop{}
	if ok, _ := c.Has("anything"); ok {
		t.Error("noop cache should never hit")
	}
	if err := c.Save(Entry{Fingerprint: "x"}); err != nil {
		t.Errorf("noop save should be a no-op, got %v", err)
	}
}

func TestOpenDisabledReturnsNoop(t *testing.T) {
	c, err := Open(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, isNoop := c.(Noop); !isNoop {
		t.Errorf("disabled cache should be Noop, got %T", c)
	}
}
