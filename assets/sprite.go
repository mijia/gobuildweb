package assets

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/mijia/gobuildweb/loggers"
)

type _Sprite struct {
	_Asset
	name       string
	fullpath   string
	pixelRatio int
}

type _ImageItem struct {
	_FileItem
	image.Image
}

func Sprite(config Config, entry string, name string, fullpath string) _Sprite {
	pixelRatio := 1
	if index := strings.LastIndex(name, "@"); index != -1 {
		switch name[index:] {
		case "@2x":
			pixelRatio = 2
		case "@3x":
			pixelRatio = 3
		}
		name = fmt.Sprintf("%s_%dx", name[:index], pixelRatio)
	}
	return _Sprite{
		_Asset:     _Asset{config, entry},
		name:       name,
		fullpath:   fullpath,
		pixelRatio: pixelRatio,
	}
}

func (s _Sprite) Build(isProduction bool) error {
	targetFolder := fmt.Sprintf("public/images/%s", s.entry)
	if err := os.MkdirAll(targetFolder, os.ModePerm|os.ModeDir); err != nil {
		return fmt.Errorf("Cannot mkdir %s, %v", targetFolder, err)
	}

	if imageItems, err := s.getImages(s.fullpath); err != nil {
		return err
	} else {
		files := make([]*os.File, len(imageItems))
		images := make([]_ImageItem, len(imageItems))
		width, height := 0, 0
		for i, imgItem := range imageItems {
			if file, err := os.Open(imgItem.fullpath); err != nil {
				return fmt.Errorf("Cannot open image file: %s, %v", imgItem.fullpath, err)
			} else {
				if image, _, err := image.Decode(file); err != nil {
					return err
				} else {
					files[i] = file
					images[i] = _ImageItem{imgItem, image}
					height += image.Bounds().Dy()
					if width < image.Bounds().Dx() {
						width = image.Bounds().Dx()
					}
				}
			}
		}

		spriteImage := image.NewNRGBA(image.Rect(0, 0, width, height))
		yOffset := 0
		for i := range images {
			files[i].Close()
			newBounds := image.Rect(0, yOffset, images[i].Bounds().Dx(), yOffset+images[i].Bounds().Dy())
			draw.Draw(spriteImage, newBounds, images[i], image.Point{0, 0}, draw.Src)
			yOffset += images[i].Bounds().Dy()
		}
		return s.save(spriteImage, images)
	}
	return nil
}

func (s _Sprite) save(spriteImg image.Image, items []_ImageItem) error {
	targetFolder := fmt.Sprintf("public/images/%s", s.entry)
	target := path.Join(targetFolder, s.name+".png")
	if file, err := os.Create(target); err != nil {
		return fmt.Errorf("Cannot create sprite file %s, %v", target, err)
	} else {
		defer file.Close()
		if err := png.Encode(file, spriteImg); err != nil {
			return nil
		}
		target = s.addFingerPrint(targetFolder, s.name+".png")
		loggers.Succ("[Sprite][%s] Saved sprite image: %s", s.entry, target)

		// generate the stylus file
		stylus := "assets/stylesheets/sprites"
		if err := os.MkdirAll(stylus, os.ModePerm|os.ModeDir); err != nil {
			return fmt.Errorf("Cannot mkdir %s, %v", stylus, err)
		}
		stylus = fmt.Sprintf("assets/stylesheets/sprites/%s_%s.styl", s.entry, s.name)
		if stylusFile, err := os.Create(stylus); err != nil {
			return fmt.Errorf("Cannot create the stylus file for sprite %s, %v", stylus, err)
		} else {
			defer stylusFile.Close()

			spriteEntry := SpriteEntry{
				Entry:   s.entry,
				Name:    s.name,
				Url:     fmt.Sprintf("%s/images/%s/%s", s.config.UrlPrefix, s.entry, filepath.Base(target)),
				Sprites: make([]SpriteImage, len(items)),
			}
			lastHeight := 0
			for i, image := range items {
				name := image.name
				name = name[:len(name)-len(filepath.Ext(name))]
				width, height := image.Bounds().Dx(), image.Bounds().Dy()
				if width%s.pixelRatio != 0 || height%s.pixelRatio != 0 {
					loggers.Warn("You have images cannot be adjusted by the pixel ratio, %s, bounds=%v, pixelRatio=%d",
						image.fullpath, image.Bounds(), s.pixelRatio)
				}
				spriteEntry.Sprites[i] = SpriteImage{
					Name:   fmt.Sprintf("%s-%s", s.name, name),
					X:      0,
					Y:      -1 * lastHeight,
					Width:  width / s.pixelRatio,
					Height: height / s.pixelRatio,
				}
				lastHeight += height / s.pixelRatio
			}
			if err := tmSprites.Execute(stylusFile, spriteEntry); err != nil {
				return fmt.Errorf("Cannot generate stylus for sprites %s, %v", spriteEntry, err)
			}
		}
	}
	return nil
}

type SpriteEntry struct {
	Entry   string
	Name    string
	Url     string
	Sprites []SpriteImage
}

type SpriteImage struct {
	Name   string
	X      int
	Y      int
	Width  int
	Height int
}

var tmplSprites = `{{$EntryName := .Entry }}
{{range .Sprites}}${{$EntryName}}-{{.Name}} = {{.X}}px {{.Y}}px {{.Width}}px {{.Height}}px
{{end}}
{{.Entry}}-{{.Name}}($sprite)
  background-image url("{{.Url}}")
  background-position $sprite[0] $sprite[1]
  width $sprite[2]
  height $sprite[3]
`
var tmSprites *template.Template

func init() {
	tmSprites = template.Must(template.New("sprites").Parse(tmplSprites))
}
