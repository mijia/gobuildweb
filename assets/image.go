package assets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mijia/gobuildweb/loggers"
)

type _ImageItem struct {
	name     string
	fullpath string
}

type _ImageLibrary struct {
	_Asset
}

func ImageLibrary(config Config, entry string) _ImageLibrary {
	return _ImageLibrary{
		_Asset: _Asset{config, entry},
	}
}

func (il _ImageLibrary) Build(isProduction bool) error {
	folderName := fmt.Sprintf("assets/images/%s", il.entry)
	if exist, err := il.checkFile(folderName, false); !exist {
		return err
	}
	targetFolder := fmt.Sprintf("public/images/%s", il.entry)
	if err := ResetDir(targetFolder, true); err != nil {
		return err
	}

	// copy the single image files
	if imageItems, err := il.getImages(folderName); err != nil {
		return err
	} else {
		for _, imgItem := range imageItems {
			target := fmt.Sprintf("public/images/%s/%s", il.entry, imgItem.name)
			if err := il.copyFile(target, imgItem.fullpath); err != nil {
				return err
			}
			target = il.addFingerPrint(targetFolder, imgItem.name)
			loggers.Succ("[ImageLibrary][%s] Saved images: %s", il.entry, target)
		}
	}

	// check if we have sprite folders under assets
	return il.buildSprites(il.entry)
}

func (il _ImageLibrary) buildSprites(entry string) error {

	return nil
}

func (il _ImageLibrary) getImages(folderName string) ([]_ImageItem, error) {
	allowedExts := make(map[string]struct{})
	for _, ext := range il.config.ImageExts {
		allowedExts[ext] = struct{}{}
	}
	items := make([]_ImageItem, 0)
	err := filepath.Walk(folderName, func(fname string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if _, ok := allowedExts[filepath.Ext(fname)]; ok {
				items = append(items, _ImageItem{info.Name(), fname})
			}
		}
		if fname != folderName && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return items, err
}
