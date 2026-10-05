package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Catalog is the global brand asset library (assets.yaml).
type Catalog struct {
	Version int     `yaml:"version" json:"version"`
	Storage Storage `yaml:"storage" json:"storage"`
	Brands  []Brand `yaml:"brands" json:"brands"`
}

type Storage struct {
	Root           string `yaml:"root" json:"root"`
	PreviewDirname string `yaml:"preview_dirname" json:"preview_dirname"`
	ThumbDirname   string `yaml:"thumb_dirname" json:"thumb_dirname"`
}

type Brand struct {
	ID          string  `yaml:"id" json:"id"`
	Name        string  `yaml:"name" json:"name"`
	Description string  `yaml:"description" json:"description"`
	CreatedAt   string  `yaml:"created_at" json:"created_at"`
	UpdatedAt   string  `yaml:"updated_at" json:"updated_at"`
	Visuals     Visuals `yaml:"visuals" json:"visuals"`
	Colors      Colors  `yaml:"colors" json:"colors"`
}

type Visuals struct {
	Logo          LogoAsset      `yaml:"logo" json:"logo"`
	ProductImages []ProductImage `yaml:"product_images" json:"product_images"`
}

type LogoAsset struct {
	Originals []LogoFile `yaml:"originals" json:"originals"`
	Preview   FileRef    `yaml:"preview" json:"preview"`
	Meta      LogoMeta   `yaml:"meta" json:"meta"`
}

type LogoFile struct {
	Path      string `yaml:"path" json:"path"`
	Format    string `yaml:"format" json:"format"`
	Preferred bool   `yaml:"preferred" json:"preferred"`
}

type FileRef struct {
	Path   string `yaml:"path" json:"path"`
	Width  int    `yaml:"width,omitempty" json:"width,omitempty"`
	Height int    `yaml:"height,omitempty" json:"height,omitempty"`
}

type LogoMeta struct {
	TransparentBackground bool    `yaml:"transparent_background" json:"transparent_background"`
	MinWhitespaceRatio    float64 `yaml:"min_whitespace_ratio" json:"min_whitespace_ratio"`
	SafeMarginPx          int     `yaml:"safe_margin_px" json:"safe_margin_px"`
	SafeMarginRatio       float64 `yaml:"safe_margin_ratio" json:"safe_margin_ratio"`
}

type ProductImage struct {
	ID    string   `yaml:"id" json:"id"`
	Path  string   `yaml:"path" json:"path"`
	Thumb FileRef  `yaml:"thumb" json:"thumb"`
	Tags  []string `yaml:"tags" json:"tags"`
	Alt   string   `yaml:"alt" json:"alt"`
}

type Colors struct {
	Primary    string `yaml:"primary" json:"primary"`
	Secondary1 string `yaml:"secondary_1" json:"secondary_1"`
	Secondary2 string `yaml:"secondary_2" json:"secondary_2"`
	Text       string `yaml:"text" json:"text"`
	Background string `yaml:"background" json:"background"`
}

// UploadRecord is one file accepted by the studio uploader.
type UploadRecord struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // document | image | video | audio | logo
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	URL         string    `json:"url"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	BrandID     string    `json:"brand_id,omitempty"`
	UploadedAt  time.Time `json:"uploaded_at"`
}

// DefaultCatalogPath is the repo assets.yaml.
func DefaultCatalogPath() string {
	return filepath.Join("internal", "service", "models", "assets", "assets.yaml")
}

// LoadCatalog reads assets.yaml.
func LoadCatalog(path string) (*Catalog, error) {
	if path == "" {
		path = DefaultCatalogPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// BrandSummary is a compact list item for the studio UI.
type BrandSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Primary     string `json:"primary,omitempty"`
	Secondary1  string `json:"secondary_1,omitempty"`
}

func (c *Catalog) Summaries() []BrandSummary {
	if c == nil {
		return nil
	}
	out := make([]BrandSummary, 0, len(c.Brands))
	for _, b := range c.Brands {
		out = append(out, BrandSummary{
			ID:          b.ID,
			Name:        b.Name,
			Description: strings.TrimSpace(b.Description),
			Primary:     b.Colors.Primary,
			Secondary1:  b.Colors.Secondary1,
		})
	}
	return out
}

// ResolveBrandRoot returns host directory for a brand's files.
func (c *Catalog) ResolveBrandRoot(projectRoot, brandID string) string {
	root := "assets/brands"
	if c != nil && c.Storage.Root != "" {
		root = c.Storage.Root
	}
	return filepath.Join(projectRoot, root, brandID)
}

// FindBrand returns a brand by id (case-sensitive).
func (c *Catalog) FindBrand(id string) (*Brand, error) {
	id = strings.TrimSpace(id)
	if c == nil || id == "" {
		return nil, fmt.Errorf("brand id required")
	}
	for i := range c.Brands {
		if c.Brands[i].ID == id {
			return &c.Brands[i], nil
		}
	}
	return nil, fmt.Errorf("brand not found: %s", id)
}

// ResolveLogoPNG returns host path + bytes for a brand logo preview (PNG preferred).
func (c *Catalog) ResolveLogoPNG(projectRoot, brandID string) (hostPath string, data []byte, meta LogoMeta, err error) {
	b, err := c.FindBrand(brandID)
	if err != nil {
		return "", nil, LogoMeta{}, err
	}
	root := c.ResolveBrandRoot(projectRoot, brandID)
	meta = b.Visuals.Logo.Meta
	candidates := []string{}
	if p := strings.TrimSpace(b.Visuals.Logo.Preview.Path); p != "" {
		candidates = append(candidates, filepath.Join(root, p))
	}
	for _, o := range b.Visuals.Logo.Originals {
		if strings.EqualFold(o.Format, "png") || strings.HasSuffix(strings.ToLower(o.Path), ".png") {
			candidates = append(candidates, filepath.Join(root, o.Path))
		}
	}
	// Common fallbacks when YAML paths are incomplete but files exist on disk.
	candidates = append(candidates,
		filepath.Join(root, "logos", "previews", "logo.png"),
		filepath.Join(root, "logos", "logo.png"),
	)
	seen := map[string]struct{}{}
	for _, path := range candidates {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		raw, rerr := os.ReadFile(path)
		if rerr != nil || len(raw) < 32 {
			continue
		}
		return path, raw, meta, nil
	}
	return "", nil, meta, fmt.Errorf("logo png not found for brand %s", brandID)
}

// SaveUpload writes bytes under assets/uploads/{brandOrmisc}/{kind}/.
func SaveUpload(projectRoot, brandID, kind, filename string, data []byte) (UploadRecord, error) {
	kind = normalizeKind(kind)
	safeName := filepath.Base(filename)
	if safeName == "" || safeName == "." || safeName == ".." {
		return UploadRecord{}, fmt.Errorf("invalid filename")
	}
	brand := brandID
	if brand == "" {
		brand = "_inbox"
	}
	relDir := filepath.Join("assets", "uploads", brand, kind)
	dir := filepath.Join(projectRoot, relDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return UploadRecord{}, err
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	id := stamp + "-" + strings.ReplaceAll(safeName, " ", "_")
	abs := filepath.Join(dir, id)
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return UploadRecord{}, err
	}
	rel := filepath.ToSlash(filepath.Join(relDir, id))
	return UploadRecord{
		ID:         id,
		Kind:       kind,
		Name:       safeName,
		Path:       rel,
		URL:        "/media/" + rel,
		Size:       int64(len(data)),
		BrandID:    brandID,
		UploadedAt: time.Now().UTC(),
	}, nil
}

func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "doc", "document", "script", "text":
		return "document"
	case "img", "image", "logo", "product":
		return "image"
	case "vid", "video":
		return "video"
	case "aud", "audio", "music":
		return "audio"
	default:
		if kind == "" {
			return "image"
		}
		return strings.ToLower(kind)
	}
}

// FindUploadByID locates a previously saved upload by its id (file basename).
func FindUploadByID(projectRoot, id string) (string, error) {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" || id == "." || id == ".." {
		return "", fmt.Errorf("invalid upload id")
	}
	root := filepath.Join(projectRoot, "assets", "uploads")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return "", fmt.Errorf("upload not found: %s", id)
	}
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == id {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("upload not found: %s", id)
	}
	return found, nil
}
