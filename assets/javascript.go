package assets

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mijia/gobuildweb/loggers"
)

type _JavaScript struct {
	_Asset
}

func JavaScript(config Config, entry string) _JavaScript {
	return _JavaScript{
		_Asset: _Asset{config, entry},
	}
}

func (js _JavaScript) Build(isProduction bool) error {
	if os.Getenv("NODE_ENV") == "production" {
		isProduction = true
	}

	assetEntry, ok := js.config.getAssetEntry(js.entry)
	if !ok {
		return nil
	}

	target := fmt.Sprintf("public/javascripts/%s.js", js.entry)
	filename := fmt.Sprintf("assets/javascripts/%s.js", js.entry)
	isCoffee := false
	if exist, _ := js.checkFile(filename, true); !exist {
		filename = fmt.Sprintf("assets/javascripts/%s.coffee", js.entry)
		if exist, err := js.checkFile(filename, true); !exist {
			return err
		}
		isCoffee = true
	}

	// * Maybe it's a template using images, styles assets links
	// TODO

	// * run browserify
	params := []string{filename}
	for _, require := range assetEntry.Requires {
		params = append(params, "--require", require)
	}
	for _, external := range assetEntry.Externals {
		if anEntry, ok := js.config.getAssetEntry(external); ok {
			for _, require := range anEntry.Requires {
				params = append(params, "--external", require)
			}
		}
	}
	for _, opt := range assetEntry.BundleOpts {
		params = append(params, opt)
	}
	if isCoffee {
		params = append(params, "--transform", "coffeeify")
	} else {
		params = append(params, "--transform", "babelify")
	}
	params = append(params, "--transform", "envify")
	if isProduction {
		params = append(params, "-g", "uglifyify")
	} else {
		params = append(params, "--debug")
	}
	params = append(params, "--outfile", target)
	cmd := exec.Command("./node_modules/browserify/bin/cmd.js", params...)
	loggers.Debug("[JavaScript][%s] Building asset: %s, %v", js.entry, filename, cmd.Args)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = js.getEnv(isProduction)
	if err := cmd.Run(); err != nil {
		loggers.Error("[JavaScript][%s] Error when building asset %v, %v", js.entry, cmd.Args, err)
		return err
	}

	// * generate the hash, clear old bundle, move to target
	target = js.addFingerPrint("public/javascripts", js.entry+".js")
	loggers.Succ("[JavaScript][%s] Saved assset: %s", js.entry, target)

	return nil
}
