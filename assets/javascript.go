package assets

import (
	"os"
	"os/exec"
	"path"

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

	suffixJs, suffixCoffee, originDir, targetDir := ".js", ".coffee", "assets/javascripts", "public/javascripts"
	filename, outfile := path.Join(originDir, js.entry+suffixJs), path.Join(targetDir, js.entry+suffixJs)
	isCoffee := false
	if exist, _ := js.checkFile(filename, true); !exist {
		filename = path.Join(originDir, js.entry+suffixCoffee)
		if exist, err := js.checkFile(filename, true); !exist {
			return err
		}
		isCoffee = true
	}

	// * generate the hash, just loaded if not change
	outTarget := js.traverEntryFingerPrint(originDir, targetDir, js.entry, filename, suffixJs)
	mapping := js._Asset.getJsonAssetsMapping()
	if targetName, ok := mapping[outfile[len("public/"):]]; ok {
		if targetName == outTarget[len("public/"):] {
			loggers.Succ("[JavaScript][%s] Loaded assset: %s", js.entry, outTarget)
			return nil
		}
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
		params = append(params, "--transform", "[", "babelify", "--presets", "[", "es2015", "react", "]", "]")
	}
	params = append(params, "--transform", "envify")
	if isProduction {
		params = append(params, "-g", "uglifyify")
	} else {
		params = append(params, "--debug")
	}
	params = append(params, "--outfile", outfile)
	cmd := exec.Command("./node_modules/browserify/bin/cmd.js", params...)
	loggers.Debug("[JavaScript][%s] Building asset: %s, %v", js.entry, filename, cmd.Args)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = js.getEnv(isProduction)
	if err := cmd.Run(); err != nil {
		loggers.Error("[JavaScript][%s] Error when building asset %v, %v", js.entry, cmd.Args, err)
		return err
	}

	//clear old bundle, move to target
	js._Asset.removeOldFile(targetDir, js.entry+suffixJs)
	if err := os.Rename(outfile, outTarget); err != nil {
		loggers.Error("rename file error, %v", err)
	}

	// target = js.addFingerPrint("public/javascripts", js.entry+".js")
	loggers.Succ("[JavaScript][%s] Saved assset: %s", js.entry, outTarget)
	return nil
}
