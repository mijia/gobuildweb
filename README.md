Build a Golang web application
-----

An in-house tool for building a Golang web application.

Features:

+ Server/App auto reloading
+ Assets build which depends on `npm` and `browserify`, provides supports for 
    + ES6 Supports via 6to5
    + React JSX file preprocessing using 6to5 jsx transformation
    + CoffeeScript support
    + Sprite images generation, supporting different pixel ratio, sprite@1x, sprite@2x, sprite@3x
    + Stylus css preprocessing
    + Assets uglify and minimization
    + Assets fingerprint
+ Cross Compilation
    + for this, please refer to http://dave.cheney.net/2012/09/08/an-introduction-to-cross-compilation-with-go

MIT License
-----