package crawl

import (
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func localRootSeedURLs(entryURL, root string) ([]string, error) {
	parsed, err := url.Parse(entryURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("entry URL must include scheme and host: %q", entryURL)
	}

	basePath := localRootBasePath(parsed.Path)
	seen := map[string]struct{}{}
	seeds := make([]string, 0)
	err = filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isHTMLPath(entry.Name()) {
			return nil
		}

		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		route := localHTMLRoute(filepath.ToSlash(rel))
		seed := *parsed
		seed.Path = joinURLPath(basePath, route)
		seed.RawPath = ""
		seed.ForceQuery = false
		seed.RawQuery = ""
		seed.Fragment = ""

		normalized := normalizeURL(&seed).String()
		if _, ok := seen[normalized]; ok {
			return nil
		}
		seen[normalized] = struct{}{}
		seeds = append(seeds, normalized)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(seeds)
	return seeds, nil
}

func isHTMLPath(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func localRootBasePath(entryPath string) string {
	if entryPath == "" {
		return "/"
	}
	if strings.HasSuffix(entryPath, "/") {
		return entryPath
	}
	return path.Dir(entryPath) + "/"
}

func localHTMLRoute(rel string) string {
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	switch strings.ToLower(path.Base(rel)) {
	case "index.html", "index.htm":
		dir := path.Dir(rel)
		if dir == "." {
			return ""
		}
		return strings.TrimPrefix(dir, "/") + "/"
	default:
		return rel
	}
}

func joinURLPath(basePath, rel string) string {
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	if rel == "" {
		return basePath
	}

	wantTrailingSlash := strings.HasSuffix(rel, "/")
	joined := path.Join(basePath, rel)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	if wantTrailingSlash && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}
