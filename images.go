package main

import (
	"fmt"
	"html"
	"html/template"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// imageFile returns the variant path for a given image name, extension, and width.
// The extension should not include a leading dot.
func imageFile(name, ext string, width int) string {
	return fmt.Sprintf("img/%s-%d.%s", name, width, ext)
}

// processImages walks the image manifests and generates any missing variant
// files on disk. It is idempotent: if the output file already exists, it is
// skipped.
func processImages(manifestDir string, images []ImageManifest) error {
	// All variants live in <manifestDir>/img, regardless of image or width.
	imgDir := filepath.Join(manifestDir, "img")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %s: %s", imgDir, err)
	}

	for _, im := range images {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(im.Src)), ".")
		srcPath := filepath.Join(manifestDir, im.Src)

		src, err := imaging.Open(srcPath)
		if err != nil {
			return fmt.Errorf("opening %s: %s", srcPath, err)
		}

		for _, w := range im.Widths {
			outPath := filepath.Join(manifestDir, imageFile(im.Name, ext, w))

			// Skip if the variant already exists. Stat succeeding (err == nil)
			// means the file is there; any error is treated as "regenerate."
			// We only check for existence — contents are not validated, so a
			// stale or corrupt variant on disk will be left alone.
			if _, err := os.Stat(outPath); err == nil {
				continue
			}

			resized := imaging.Resize(src, w, 0, imaging.Lanczos)
			if err := imaging.Save(resized, outPath); err != nil {
				return fmt.Errorf("saving %s: %s", outPath, err)
			}
		}
	}

	return nil
}

// buildImageTable constructs the name→manifest lookup table, populating each
// entry's Width and Height by reading the source image's header.
func buildImageTable(manifestDir string, images []ImageManifest) (map[string]ImageManifest, error) {
	table := make(map[string]ImageManifest, len(images))
	for _, im := range images {
		srcPath := filepath.Join(manifestDir, im.Src)
		f, err := os.Open(srcPath)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %s", srcPath, err)
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding config for %s: %s", srcPath, err)
		}
		im.width = cfg.Width
		im.height = cfg.Height
		table[im.Name] = im
	}
	return table, nil
}

// imageFunc returns the template function used to render an <img> tag with a
// srcset built from the manifest's declared widths.
func imageFunc(table map[string]ImageManifest) func(name, alt, sizes string) (template.HTML, error) {
	return func(name, alt, sizes string) (template.HTML, error) {
		im, ok := table[name]
		if !ok {
			return "", fmt.Errorf("unknown image name: %q", name)
		}

		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(im.Src)), ".")

		if sizes == "" {
			sizes = "100vw"
		}

		// Build srcset and pick the largest width as the default src.
		var (
			srcset     []string
			largestW   int
			largestURL string
		)
		for _, w := range im.Widths {
			url := "/" + imageFile(im.Name, ext, w)
			srcset = append(srcset, fmt.Sprintf("%s %dw", url, w))
			if w > largestW {
				largestW = w
				largestURL = url
			}
		}

		// Default attributes are the largest variant's intrinsic dims.
		defW := largestW
		defH := defW * im.height / im.width

		out := fmt.Sprintf(
			`<img src="%s" srcset="%s" sizes="%s" width="%d" height="%d" alt="%s">`,
			html.EscapeString(largestURL),
			html.EscapeString(strings.Join(srcset, ", ")),
			html.EscapeString(sizes),
			defW,
			defH,
			html.EscapeString(alt),
		)
		return template.HTML(out), nil
	}
}
