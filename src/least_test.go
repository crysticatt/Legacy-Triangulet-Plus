package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLeastTarget(t *testing.T) {
	// own one of everything but "Dark Web" (5) and "Meteor" (1): a missing trian has 0 so it wins
	var inv []InvItem
	for _, b := range builtinTrians {
		q := 3
		if b.Name == "Dark Web" {
			q = 5
		}
		inv = append(inv, InvItem{Name: b.Name, Qty: q})
	}
	// remove Meteor: it becomes the only trian with 0
	for i, it := range inv {
		if it.Name == "Meteor" {
			inv = append(inv[:i], inv[i+1:]...)
			break
		}
	}
	if n, c := leastTarget(inv, nil, ""); n != "Meteor" || c != "Space" {
		t.Fatalf("got %s/%s", n, c)
	}
	// wishlist restricts the pool
	if n, _ := leastTarget(inv, []string{"dark web", "Diamond Hat"}, ""); n != "Diamond Hat" {
		t.Fatalf("wishlist got %s", n)
	}
	// ties are random but sticky
	for i := range inv {
		inv[i].Qty = 2
	}
	inv = append(inv, InvItem{Name: "Meteor", Qty: 2})
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		n, _ := leastTarget(inv, nil, "")
		seen[n] = true
	}
	if len(seen) < 5 {
		t.Fatalf("ties not random: %d", len(seen))
	}
	if n, _ := leastTarget(inv, nil, "Pink Knight"); n != "Pink Knight" {
		t.Fatalf("pick not kept: %s", n)
	}
	if n, _ := leastTarget(inv, []string{"nope"}, ""); n != "" {
		t.Fatalf("empty wishlist: %s", n)
	}
}

func TestWebPageHasTrians(t *testing.T) {
	if bytes.Contains(webPage, []byte("__TRIANS__")) || !bytes.Contains(webPage, []byte(`"n":"Dark Web"`)) {
		t.Fatal("trian list not filled into the dashboard")
	}
}

// The dashboard script must at least parse (a typo once broke the whole Mac dashboard).
func TestDashboardScriptParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	i := bytes.Index(webPage, []byte("<script>"))
	j := bytes.LastIndex(webPage, []byte("</script>"))
	if i < 0 || j < i {
		t.Fatal("no script in dashboard")
	}
	f := filepath.Join(t.TempDir(), "dash.js")
	if err := os.WriteFile(f, webPage[i+8:j], 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", f).CombinedOutput(); err != nil {
		t.Fatalf("dashboard script does not parse: %s", out)
	}
}
