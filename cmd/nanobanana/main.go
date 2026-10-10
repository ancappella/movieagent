// Command nanobanana generates an image with Gemini Nano Banana Pro.
//
//	GEMINI_API_KEY=... go run ./cmd/nanobanana -p "a lemon tea poster, no text"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"movieagent/internal/config"
	"movieagent/internal/service/providers/gemini"
)

func main() {
	_ = config.LoadDotEnv(".env")
	ensureProxy()

	prompt := flag.String("p", "", "image prompt")
	outPath := flag.String("o", "tmp/nano-banana.png", "output path")
	model := flag.String("m", "nano-banana-pro", "model id / alias")
	aspect := flag.String("aspect", "3:4", "aspect ratio")
	size := flag.String("size", "1K", "image size: 1K / 2K")
	refPath := flag.String("ref", "", "optional reference image path")
	flag.Parse()

	text := strings.TrimSpace(*prompt)
	if text == "" && flag.NArg() > 0 {
		text = strings.TrimSpace(strings.Join(flag.Args(), " "))
	}
	if text == "" {
		text = "A soft pastel iced lemon tea marketing poster, product centered, no text, clean studio light"
	}

	svc := gemini.NewImageService(gemini.Config{Model: *model})
	in := gemini.GenerateInput{
		Prompt:      text,
		AspectRatio: *aspect,
		ImageSize:   *size,
	}
	if *refPath != "" {
		raw, err := os.ReadFile(*refPath)
		if err != nil {
			fatal(err)
		}
		in.RefImage = raw
		in.RefMIME = ""
	}

	fmt.Fprintf(os.Stderr, "model=%s aspect=%s size=%s proxy=%s\n",
		gemini.ResolveImageModel(*model), *aspect, *size, proxyHint())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := svc.Generate(ctx, in)
	if err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outPath, out.Data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("ok model=%s mime=%s bytes=%d path=%s\n", out.Model, out.MIME, len(out.Data), *outPath)
}

func ensureProxy() {
	if os.Getenv("GEMINI_HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" {
		return
	}
	os.Setenv("GEMINI_HTTP_PROXY", "http://127.0.0.1:7897")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
}

func proxyHint() string {
	for _, k := range []string{"GEMINI_HTTP_PROXY", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "(auto)"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "nanobanana:", err)
	os.Exit(1)
}
