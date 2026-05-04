package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sampleJPEG is the path used as a fixture across tests. It is generated on
// first use if missing, so the repo doesn't need to track a binary fixture.
const sampleJPEG = "testdata/sample.jpg"

// sampleW, sampleH are the source dimensions of the generated fixture.
const (
	sampleW = 200
	sampleH = 100
)

func ensureSample(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(sampleJPEG); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(sampleJPEG), 0o755); err != nil {
		t.Fatalf("mkdir testdata: %s", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, sampleW, sampleH))
	for y := 0; y < sampleH; y++ {
		for x := 0; x < sampleW; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	f, err := os.Create(sampleJPEG)
	if err != nil {
		t.Fatalf("create sample: %s", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode sample: %s", err)
	}
}

// copyFile copies src to dst, creating parent dirs.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir: %s", err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %s", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create dst: %s", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy: %s", err)
	}
}

func TestImageFile(t *testing.T) {
	cases := []struct {
		name  string
		ext   string
		width int
		want  string
	}{
		{"spring-hike", "jpg", 800, "img/spring-hike-800.jpg"},
		{"logo", "png", 400, "img/logo-400.png"},
		{"a", "jpg", 1, "img/a-1.jpg"},
	}
	for _, tc := range cases {
		got := imageFile(tc.name, tc.ext, tc.width)
		if got != tc.want {
			t.Errorf("imageFile(%q,%q,%d) = %q, want %q", tc.name, tc.ext, tc.width, got, tc.want)
		}
	}
}

func TestLoadManifestImages(t *testing.T) {
	ensureSample(t)
	dir := t.TempDir()
	copyFile(t, sampleJPEG, filepath.Join(dir, "originals/sample.jpg"))

	if err := os.WriteFile(filepath.Join(dir, "layout.html"), []byte(`{{define "x"}}{{end}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	man := Manifest{
		Layouts: []string{"layout.html"},
		Images: []ImageManifest{
			{Name: "hero", Src: "originals/sample.jpg", Widths: []int{200, 400}},
		},
	}
	byts, err := json.Marshal(man)
	if err != nil {
		t.Fatalf("marshal: %s", err)
	}
	manPath := filepath.Join(dir, "smolmanifest.json")
	if err := os.WriteFile(manPath, byts, 0o644); err != nil {
		t.Fatalf("write manifest: %s", err)
	}

	loaded, _, err := loadManifest(manPath, dir)
	if err != nil {
		t.Fatalf("loadManifest: %s", err)
	}
	for _, p := range []string{"/img/hero-200.jpg", "/img/hero-400.jpg"} {
		r, ok := loaded.Routes[p]
		if !ok {
			t.Fatalf("expected route %q to exist", p)
		}
		if r.StaticPath != p {
			t.Errorf("route %q StaticPath = %q, want %q", p, r.StaticPath, p)
		}
		if !strings.HasPrefix(r.ContentType, "image/jpeg") {
			t.Errorf("route %q ContentType = %q, want image/jpeg", p, r.ContentType)
		}
	}
}

func TestProcessImagesIdempotent(t *testing.T) {
	ensureSample(t)
	dir := t.TempDir()
	copyFile(t, sampleJPEG, filepath.Join(dir, "originals/sample.jpg"))

	images := []ImageManifest{
		{Name: "hero", Src: "originals/sample.jpg", Widths: []int{50, 100}},
	}

	if err := processImages(dir, images); err != nil {
		t.Fatalf("processImages: %s", err)
	}

	expected := []string{
		filepath.Join(dir, "img/hero-50.jpg"),
		filepath.Join(dir, "img/hero-100.jpg"),
	}
	for _, p := range expected {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected variant %q to exist: %s", p, err)
		}
	}

	// Capture mtimes and re-run; files should not be rewritten.
	before := map[string]int64{}
	for _, p := range expected {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		before[p] = st.ModTime().UnixNano()
	}

	if err := processImages(dir, images); err != nil {
		t.Fatalf("processImages re-run: %s", err)
	}

	for _, p := range expected {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.ModTime().UnixNano() != before[p] {
			t.Errorf("variant %q was rewritten on idempotent re-run", p)
		}
	}
}

func TestImageFunc(t *testing.T) {
	ensureSample(t)
	dir := t.TempDir()
	copyFile(t, sampleJPEG, filepath.Join(dir, "originals/sample.jpg"))

	images := []ImageManifest{
		{Name: "hero", Src: "originals/sample.jpg", Widths: []int{100, 200}},
	}
	table, err := buildImageTable(dir, images)
	if err != nil {
		t.Fatal(err)
	}
	fn := imageFunc(table)

	t.Run("default sizes", func(t *testing.T) {
		got, err := fn("hero", "View from the ridge", "")
		if err != nil {
			t.Fatal(err)
		}
		s := string(got)
		for _, want := range []string{
			`src="/img/hero-200.jpg"`,
			`/img/hero-100.jpg 100w`,
			`/img/hero-200.jpg 200w`,
			`sizes="100vw"`,
			`width="200"`,
			`height="100"`, // sampleH * 200/sampleW = 100
			`alt="View from the ridge"`,
		} {
			if !strings.Contains(s, want) {
				t.Errorf("output missing %q: %s", want, s)
			}
		}
	})

	t.Run("custom sizes", func(t *testing.T) {
		got, err := fn("hero", "alt", "(max-width: 600px) 100vw, 50vw")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), `sizes="(max-width: 600px) 100vw, 50vw"`) {
			t.Errorf("custom sizes not present: %s", got)
		}
	})

	t.Run("alt escaped", func(t *testing.T) {
		got, err := fn("hero", `she said "hi" & <bye>`, "")
		if err != nil {
			t.Fatal(err)
		}
		s := string(got)
		for _, want := range []string{`&#34;hi&#34;`, `&amp;`, `&lt;bye&gt;`} {
			if !strings.Contains(s, want) {
				t.Errorf("alt not html-escaped, missing %q: %s", want, s)
			}
		}
		for _, bad := range []string{`"hi"`, `<bye>`} {
			if strings.Contains(s, bad) {
				t.Errorf("alt should not contain raw %q: %s", bad, s)
			}
		}
	})

	t.Run("unknown name errors", func(t *testing.T) {
		if _, err := fn("nope", "alt", ""); err == nil {
			t.Errorf("expected error for unknown name")
		}
	})
}