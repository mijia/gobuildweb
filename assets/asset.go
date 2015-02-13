package assets

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Config struct {
	UrlPrefix    string   `toml:"url_prefix"`
	ImageExts    []string `toml:"image_exts"`
	Dependencies []string `toml:"deps"`
	VendorSets   []Entry  `toml:"vendor_set"`
	Entries      []Entry  `toml:"entry"`
}

func (config Config) getAssetEntry(entryName string) (Entry, bool) {
	for _, entry := range append(config.VendorSets, config.Entries...) {
		if entry.Name == entryName {
			return entry, true
		}
	}
	return Entry{}, false
}

type Entry struct {
	Name     string
	Requires []string

	// externals is a reference to other assets entry, need to expand this using
	// the other assets' requires
	Externals []string

	// sub-modules update will rebuild this module
	Dependencies []string `toml:"deps"`
	BundleOpts   []string `toml:"bundle_opts"`
}

type _Asset struct {
	config Config
	entry  string
}

func (a _Asset) checkFile(filename string, needsFile bool) (exist bool, err error) {
	if fi, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if fi.IsDir() && needsFile {
		return false, fmt.Errorf("%s is a directory", filename)
	}
	return true, nil
}

func (a _Asset) addFingerPrint(assetDir, filename string) string {
	target := path.Join(assetDir, filename)
	if file, err := os.Open(target); err == nil {
		defer file.Close()
		h := md5.New()
		if _, err := io.Copy(h, file); err == nil {
			newName := fmt.Sprintf("fp%s-%s", hex.EncodeToString(h.Sum(nil)), filename)
			newName = path.Join(assetDir, newName)
			a.removeOldFile(assetDir, filename)
			if err := os.Rename(target, newName); err == nil {
				return newName
			}
		}
	}
	return target
}

func (a _Asset) copyFile(dest, src string) error {
	if srcFile, err := os.Open(src); err != nil {
		return err
	} else {
		defer srcFile.Close()
		if destFile, err := os.Create(dest); err != nil {
			return err
		} else {
			defer destFile.Close()
			_, err := io.Copy(destFile, srcFile)
			return err
		}
	}
}

func (a _Asset) removeOldFile(dir, suffix string) error {
	return filepath.Walk(dir, func(fn string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(fn, suffix) {
			filename := info.Name()
			// FIXME: maybe we should be serious about checking this
			if strings.HasPrefix(filename, "fp") && strings.HasSuffix(filename, "-"+suffix) {
				os.Remove(fn)
			}
		}
		if fn != dir && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func (a _Asset) getEnv(isProduction bool) []string {
	env := []string{
		fmt.Sprintf("NODE_PATH=%s:./node_modules", os.Getenv("NODE_PATH")),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}
	if isProduction {
		env = append(env, "NODE_ENV=production")
	} else {
		env = append(env, "NODE_ENV=development")
	}
	return env
}

type _FileItem struct {
	name     string
	fullpath string
}

func (a _Asset) getImages(folderName string) ([]_FileItem, error) {
	allowedExts := make(map[string]struct{})
	for _, ext := range a.config.ImageExts {
		allowedExts[ext] = struct{}{}
	}
	items := make([]_FileItem, 0)
	err := filepath.Walk(folderName, func(fname string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if _, ok := allowedExts[filepath.Ext(fname)]; ok {
				items = append(items, _FileItem{info.Name(), fname})
			}
		}
		if fname != folderName && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return items, err
}

func ResetDir(dir string, rebuild bool) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("Cannot clean %s, %v", dir, err)
	}
	if rebuild {
		if err := os.MkdirAll(dir, os.ModePerm|os.ModeDir); err != nil {
			return fmt.Errorf("Cannot mkdir %s, %v", dir, err)
		}
	}
	return nil
}
