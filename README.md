Build a Golang web application, esp. the Assets
-----

A naive in-house tool for building a Golang web application and managing frontend assets. We use this tool to build full web sites.

Features:

+ This is a **intergration tool** not a **framework**.
+ Go source files and assets files watching, which will trigger
    + Assets auto rebuilding, including javascripts, stylesheets, images and sprites
    + Go test running, only if the changed package contains go test source
    + Go binary rebuiling and server/app reloading
+ Assets building which depends on `npm` and `browserify`, provides supports for 
    + ES6 transpiler via 6to5/babelify
    + React JSX file transpiler using 6to5/babelify jsx transformation
    + CoffeeScript transpiler via coffeeify
    + Sprite images generation, supporting different pixel ratio, sprite_xx@1x, sprite_xx@2x, sprite_xx@3x (go:pkg/image), and a coresponding sprite stylus funcs would be generated which we can import as the background image settings
    + Stylus css transpiler
    + Assets development and production mode via envyify (NODE_ENV)
    + Assets uglify and minimization
    + Assets fingerprints and will auto generate an assets mapping go code file in assets_gen.go
+ Distribution packing and cross compilation
    + For cross compilation in go1.4, please refer to https://github.com/davecheney/golang-crosscompile

Cmds
-----
```
-> gobuildweb
gobuildweb > Build a Golang web application.

Usage:
  run       Will watch your file changes and run the application, aka the dev mode
  dist      Build your web application for production
```

We can pass the parameters to the target binary in the run/dev mode, e.g.
> ```gobuildweb run -debug -web=:9090```

`GBW_DEBUG=1 gobuidlweb run` would log all the gobuildweb debug information such as the exec.Command params and etc.

Assets
-----
Assets are js files, css files and images files/sprite folders. The managed assets should be put into the `assets/images`, `assets/javascripts`, `assets/stylesheets` directory, and the generated assets would be inside `public/images`, `public/javascripts`, `public/stylesheets`, so please don't put non-generated files in those public folders (and we won't put any generated files into the git), but we won't touch the folder like `public/fonts` and etc.

In run/dev mode, the assets would be generated as the development version containing the source map, non-uglify and etc. In dist mode, the assets would be generated as the production version.

Project Configuration
-----

We use a `project.toml` file for the basic configuration, and in run/dev mode, if the project.toml file changes, we will auto soft reload all the go and assets dependencies and rebuild/reload the web server. Here is some detail:

```
# This is the TOML config file for project: todo_server

[package]
# package name, the target binary name would be "todo_server-0.1.GOOS.GOARCH"
name = "todo_server"
version = "0.1"
authors = ["Todo Server Ltd."]

# Ignore some tests in run/dev mode
omit_tests = []

# only take effects on run/dev mode
build_opts = ["-race"] 

# if the server is a graceful server, the tool won't wait for the process termination
is_graceful = true 

# here is the golang dependencies, :P
deps = [
    "github.com/julienschmidt/httprouter"
]

[assets]
url_prefix = "/assets"
image_exts = [".png", ".jpg", ".jpeg"]

# where the assets_gen.go would be generated
assets_mapping_pkg = "main" 

# JS Modules dependencies, would be installed by NPM
deps = [
    "react@0.12.2",
    "reflux",
    "todomvc-common",
]

    # We can define multiple JS vendor_sets, which will create different JavaScript packages
    [[assets.vendor_set]]
    name = "vendor-react"
    requires = ["react/addons", "reflux"]

    # Assets Entry defines the managed assets and also their vendor dependencies
    [[assets.entry]]
    name = "todo_app"
    externals = ["vendor-react"]

[distribution]
build_opts = []

# We may need to pack the "templates" or some other stuff into the deploy target zip pack
# The public folder will auto be packed into the zip pack
pack_extras = ["templates"]

# The cross compile targets, the running machine's target would be added automaticly
# Seems we are missing the GOARM supports for linux arm arch
cross_targets = [
    ["linux", "amd64"],
]
```

Known Issues
--------
+ We don't kown how to do browserify-shim stuff yet, like the bootstrap cannot be browserified with the NPM modules

You may ask
--------
+ Why not using grunt, gulp, webpack and etc?
    + Because we are no nodejs experts and we want a single config file and a single tool for this
+ Why not using godep, nut for the go dependency?
    + We would like to provide a fallback method, just using `go get`, but other tools can be used
+ Why not watching the go template files?
    + This seems difficult to generalize, and our own render has the ability to recompile all the templates in debug mode, so we ain't need this, :)
+ What's the purpose of `assets_gen.go`?
    + We use this mapping to do assets url reverse, since we generated the assets with fingerprints then we can write some reverse inside the html templates like `<img src="{{ assets "images/common/logo.png" }}">`

Thank you very much and any suggestions would be appreciated.

License
-----
MIT 