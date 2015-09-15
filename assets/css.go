package assets

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"path"
	"github.com/mijia/gobuildweb/loggers"
)

type _StyleSheet struct {
	_Asset
}

func StyleSheet(config Config, entry string) _StyleSheet {
	return _StyleSheet{
		_Asset: _Asset{config, entry},
	}
}

func (css _StyleSheet) Build(isProduction bool) error {
	filename := fmt.Sprintf("assets/stylesheets/%s.styl", css.entry)
	isStylus := true
	if exist, _ := css.checkFile(filename, true); !exist {
		filename = fmt.Sprintf("assets/stylesheets/%s.css", css.entry)
		if exist, err := css.checkFile(filename, true); !exist {
			return err
		} else {
			isStylus = false
		}
	}

	target := fmt.Sprintf("public/stylesheets/%s.css", css.entry)

	// * Maybe it's a template using images, styles assets links
	// TODO

	// * Maybe we need to call stylus preprocess
	if isStylus {
		params := []string{"--use", "nib", filename, "--out", "public/stylesheets"}
		if isProduction {
			params = append(params, "--compress")
		} else {
			params = append(params, "--sourcemap-inline")
		}
		// add asset plugin to change original file path to the finger printed one
		params = append(params, "--use", getStylusPluginPath())
		cmd := exec.Command("./node_modules/stylus/bin/stylus", params...)
		loggers.Debug("[CSS][%s] Building asset: %s, %v", css.entry, filename, cmd.Args)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		cmd.Env = css.getEnv(isProduction)
		if err := cmd.Run(); err != nil {
			loggers.Error("[CSS][%s] Error when building asset %v, %v", css.entry, cmd.Args, err)
			return err
		}
	} else {
		if err := css.copyFile(target, filename); err != nil {
			return err
		}
	}
	// * generate the hash, clear old bundle, move to target
	target = css.addFingerPrint("public/stylesheets", css.entry+".css")
	loggers.Succ("[CSS][%s] Saved assset: %s", css.entry, target)

	return nil
}


func getStylusPluginPath() string{
	_, file, _, _ := runtime.Caller(0)
	return path.Join(path.Dir(file), "stylus-assets.js")
}