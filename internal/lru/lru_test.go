package lru

import "testing"

func TestEviction(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if _, ok := c.Get("a"); !ok { // a is now the most recent
		t.Fatal("a missing")
	}
	c.Put("c", 3) // evicts b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b not evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatal("a evicted")
	}
	c.Put("a", 9) // the first value stays
	if v, _ := c.Get("a"); v != 1 {
		t.Fatalf("a = %d", v)
	}
}
