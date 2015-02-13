package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mijia/gobuildweb/loggers"
)

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
	return il.buildSprites(il.entry, isProduction)
}

func (il _ImageLibrary) buildSprites(entry string, isProduction bool) error {
	items := make([]_FileItem, 0)
	folderName := fmt.Sprintf("assets/images/%s", il.entry)
	err := filepath.Walk(folderName, func(fname string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && fname != folderName {
			if strings.HasPrefix(info.Name(), "sprite") {
				items = append(items, _FileItem{info.Name(), fname})
			}
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := Sprite(il.config, il.entry, item.name, item.fullpath).Build(isProduction); err != nil {
			return fmt.Errorf("[ImageLibrary][%s] Error when generating sprite, %v", il.entry, err)
		}
	}
	return nil
}
