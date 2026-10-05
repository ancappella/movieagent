package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"movieagent/internal/service/config"
	"movieagent/internal/service/sandbox"
)

func main() {
	base, err := os.MkdirTemp("", "sandbox-case-*")
	if err != nil {
		exit(err)
	}
	defer os.RemoveAll(base)

	paths, err := config.NewPaths(&base)
	if err != nil {
		exit(err)
	}
	
	provider := sandbox.NewLocalProvider(paths)

	fmt.Println("== deny: viewer cannot acquire ==")
	if err := runCase(provider, "viewer", "alice", "t1"); err == nil {
		exit(fmt.Errorf("viewer should have been denied"))
	} else {
		fmt.Println(err)
	}

	fmt.Println("== allow: agent writes, runs, and reads ==")
	if err := runCase(provider, "agent", "alice", "t1"); err != nil {
		exit(err)
	}

	fmt.Println("== allow: agent stores an image ==")
	if err := storeImageCase(provider, "alice", "t1"); err != nil {
		exit(err)
	}
}

func runCase(provider *sandbox.LocalProvider, role, userID, threadID string) error {
	if !allowSandboxExecute(role) {
		return sandbox.ErrAuthorization(role)
	}

	id, err := provider.Acquire(threadID, userID)
	if err != nil {
		return err
	}
	sb, ok := provider.Get(id)
	if !ok {
		return sandbox.ErrNotFound(id)
	}
	fmt.Printf("sandbox_id=%s\n", id)

	const virtual = "/mnt/user-data/workspace/hello.txt"
	if err := sb.WriteFile(virtual, "hello from sandbox case\n", false); err != nil {
		return err
	}
	out, err := sb.ExecuteCommand(context.Background(), "cat "+virtual, nil)
	if err != nil {
		return err
	}
	fmt.Printf("command: %s", out)
	if out != "" && out[len(out)-1] != '\n' {
		fmt.Println()
	}

	body, err := sb.ReadFile(virtual, 0, 0)
	if err != nil {
		return err
	}
	fmt.Printf("read_file: %s", body)
	return nil
}

func storeImageCase(provider *sandbox.LocalProvider, userID, threadID string) error {
	id, err := provider.Acquire(threadID, userID)
	if err != nil {
		return err
	}
	sb, ok := provider.Get(id)
	if !ok {
		return sandbox.ErrNotFound(id)
	}

	pngBytes, err := samplePNG()
	if err != nil {
		return err
	}
	const virtual = "/mnt/user-data/outputs/poster.png"
	if err := sb.WriteFile(virtual, string(pngBytes), false); err != nil {
		return err
	}
	got, err := sb.ReadFile(virtual, 0, 0)
	if err != nil {
		return err
	}
	if got != string(pngBytes) {
		return fmt.Errorf("image bytes mismatch: wrote %d, read %d", len(pngBytes), len(got))
	}

	out, err := sb.ExecuteCommand(context.Background(), "wc -c < "+virtual, nil)
	if err != nil {
		return err
	}
	fmt.Printf("sandbox_id=%s\n", id)
	fmt.Printf("stored %s bytes=%d png=%v\n", virtual, len(pngBytes), bytes.HasPrefix(pngBytes, []byte{0x89, 'P', 'N', 'G'}))
	fmt.Printf("wc: %s", out)
	if out != "" && out[len(out)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func samplePNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 220, G: 40, B: 40, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func allowSandboxExecute(role string) bool {
	return role == "agent" || role == "admin"
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
