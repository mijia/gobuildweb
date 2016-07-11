package assets

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/mijia/gobuildweb/loggers"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Config struct {
	UrlPrefix                string   `toml:"url_prefix"`
	AssetsMappingPkg         string   `toml:"assets_mapping_pkg"`
	AssetsMappingPkgRelative string   `toml:"assets_mapping_pkg_relative"`
	AssetsMappingJson        string   `toml:"assets_mapping_json"`
	ImageExts                []string `toml:"image_exts"`
	Dependencies             []string `toml:"deps"`
	VendorSets               []Entry  `toml:"vendor_set"`
	Entries                  []Entry  `toml:"entry"`
}

func (config Config) getAssetEntry(entryName string) (Entry, bool) {
	return GetEntryConfig(config, entryName)
}

func GetEntryConfig(config Config, entryName string) (Entry, bool) {
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

func (a _Asset) traverEntryFingerPrint(originDir, targetDir, entry, filename, suffix string) string {
	h, target := md5.New(), path.Join(targetDir, entry, suffix)
	if file, err := os.Open(filename); err != nil {
		return target
	} else {
		defer file.Close()
		if _, err := io.Copy(h, file); err != nil {
			return target
		}
	}
	goWalk := func(walkDir string) error {
		return filepath.Walk(walkDir, func(fn string, info os.FileInfo, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if !info.IsDir() {
				if file, err := os.Open(path.Join(fn)); err == nil {
					defer file.Close()
					if _, err := io.Copy(h, file); err != nil {
						return err
					}
				} else {
					return err
				}
			}
			return err
		})
	}
	err := goWalk(path.Join(originDir, entry))
	if err != nil && err != filepath.SkipDir {
		return target
	}
	if entryInfo, existed := GetEntryConfig(a.config, entry); existed {
		for _, dep := range entryInfo.Dependencies {
			err := goWalk(path.Join(originDir, dep))
			if err != nil && err != filepath.SkipDir {
				return target
			}
		}
	}
	newName := fmt.Sprintf("fp%s-%s", hex.EncodeToString(h.Sum(nil)), entry+suffix)
	return path.Join(targetDir, newName)
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
			parts := strings.SplitN(filename, "-", 2)
			if len(parts) == 2 && strings.HasPrefix(parts[0], "fp") && parts[1] == suffix {
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

func (a _Asset) getJsonAssetsMapping() map[string]string {
	mapping := make(map[string]string)
	filename := a.config.AssetsMappingJson
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		loggers.Error(fmt.Sprintf("Cannot decoding the assets into json, %v", err))
	} else {
		err = json.Unmarshal(data, &mapping)
		if err != nil {
			loggers.Error(fmt.Sprintf("Cannot unmarshal the assets into json, %v", err))
		}
	}
	return mapping
}
