package router

import (
	"io/fs"
	"os"
	"path"
	"testing"
)

func TestCheckoutSourcePrefersExternalStaticCheckout(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	if err := os.MkdirAll("static/checkout/custom", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("static/checkout/custom/marker.txt", []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}

	src, root, _ := checkoutSource()
	got, err := fs.ReadFile(src, path.Join(root, "custom", "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "external" {
		t.Fatalf("expected external checkout file, got %q", got)
	}
}

func TestCheckoutSourceFallsBackToEmbeddedCheckout(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	src, root, _ := checkoutSource()
	if _, err := fs.Stat(src, root); err != nil {
		t.Fatalf("expected embedded checkout fallback: %v", err)
	}
}
